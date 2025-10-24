import asyncio
import logging
import uuid
import sys
import os
import socket
import time
from concurrent import futures
from datetime import datetime
from typing import Dict, Optional, List
import grpc

sys.path.append(os.path.dirname(__file__))

import agent_manager_pb2
import agent_manager_pb2_grpc

from ..agents import AgentManager
from ..grpc_client import ContestManagerClient

logger = logging.getLogger(__name__)


class AgentInfo:
    
    def __init__(
        self,
        agent_id: str,
        contest_id: str,
        participant_id: str,
        model_name: str,
        task: asyncio.Task
    ):
        self.agent_id = agent_id
        self.contest_id = contest_id
        self.participant_id = participant_id
        self.model_name = model_name
        self.task = task
        self.status = agent_manager_pb2.AGENT_STATUS_INITIALIZING
        self.started_at = datetime.now()
        self.completed_at: Optional[datetime] = None
        self.error_message = ""
        self.current_step = "initializing"


class AgentManagerServicer(agent_manager_pb2_grpc.AgentManagerServiceServicer):
    
    def __init__(self):
        self.agent_manager = AgentManager()
        self.agents: Dict[str, AgentInfo] = {}
        self.status_subscribers: List[asyncio.Queue] = []
        self.event_loop = asyncio.new_event_loop()
        import threading
        self.loop_thread = threading.Thread(target=self._run_event_loop, daemon=True)
        self.loop_thread.start()
        logger.info("AgentManagerServicer initialized")
    
    def _update_agent_status_from_step(self, agent_info: AgentInfo, step: str):
        step_to_status = {
            "analyze_contest": agent_manager_pb2.AGENT_STATUS_ANALYZING_CONTEST,
            "select_problem": agent_manager_pb2.AGENT_STATUS_SELECTING_PROBLEM,
            "solve_problem": agent_manager_pb2.AGENT_STATUS_SOLVING_PROBLEM,
            "coding": agent_manager_pb2.AGENT_STATUS_CODING,
            "submit_solution": agent_manager_pb2.AGENT_STATUS_SUBMITTING_SOLUTION,
            "check_results": agent_manager_pb2.AGENT_STATUS_CHECKING_RESULTS,
            "monitor_contest": agent_manager_pb2.AGENT_STATUS_MONITORING_CONTEST,
            "contest_ended": agent_manager_pb2.AGENT_STATUS_CONTEST_ENDED,
        }
        
        new_status = step_to_status.get(step, agent_info.status)
        if new_status != agent_info.status:
            agent_info.status = new_status
            agent_info.current_step = step
            self._broadcast_status_update(agent_info)
            logger.info(f"Agent {agent_info.agent_id} status updated to {step}")
    
    async def _monitor_agent_status(self, agent_info: AgentInfo, agent):
        last_step = None
        
        agent_key = f"{agent_info.contest_id}_{agent_info.participant_id}"
        self.agent_manager.active_agents[agent_key] = agent
        
        try:
            logger.info(f"Starting monitoring loop for agent {agent_info.agent_id}")
            while True:
                try:
                    
                    current_step = None
                    
                    if hasattr(agent, '_current_state') and agent._current_state:
                        current_step = agent._current_state.get('current_step')
                        logger.debug(f"Agent {agent_info.agent_id} current_step from _current_state: {current_step}")
                    elif hasattr(agent, 'workflow_state') and agent.workflow_state:
                        current_step = agent.workflow_state.get('current_step')
                        logger.debug(f"Agent {agent_info.agent_id} current_step from workflow_state: {current_step}")
                    else:
                        logger.debug(f"Agent {agent_info.agent_id} has no accessible state")
                    
                    logger.debug(f"Agent {agent_info.agent_id} current_step: {current_step}, last_step: {last_step}")
                    if current_step and current_step != last_step:
                        step_mapping = {
                            'analyze_contest': agent_manager_pb2.AGENT_STATUS_ANALYZING_CONTEST,
                            'select_problem': agent_manager_pb2.AGENT_STATUS_SELECTING_PROBLEM,
                            'solve_problem': agent_manager_pb2.AGENT_STATUS_SOLVING_PROBLEM,
                            'coding': agent_manager_pb2.AGENT_STATUS_CODING,
                            'submit_solution': agent_manager_pb2.AGENT_STATUS_SUBMITTING_SOLUTION,
                            'check_results': agent_manager_pb2.AGENT_STATUS_CHECKING_RESULTS,
                            'monitor_contest': agent_manager_pb2.AGENT_STATUS_MONITORING_CONTEST,
                            'contest_ended': agent_manager_pb2.AGENT_STATUS_CONTEST_ENDED,
                            'no_problems': agent_manager_pb2.AGENT_STATUS_CONTEST_ENDED
                        }
                        
                        mapped_status = step_mapping.get(current_step)
                        if mapped_status and mapped_status != agent_info.status:
                            agent_info.status = mapped_status
                            agent_info.current_step = current_step
                            self._broadcast_status_update(agent_info)
                            last_step = current_step
                            logger.info(f"Agent {agent_info.agent_id} workflow step: {current_step} -> status {mapped_status}")
                    
                    await asyncio.sleep(2)  
                    
                except asyncio.CancelledError:
                    break
                except Exception as e:
                    logger.error(f"Error monitoring agent status: {e}")
                    await asyncio.sleep(5)
        finally:
            if agent_key in self.agent_manager.active_agents:
                del self.agent_manager.active_agents[agent_key]
    
    def _run_event_loop(self):
        asyncio.set_event_loop(self.event_loop)
        self.event_loop.run_forever()
    
    async def _run_agent_async(
        self,
        agent_info: AgentInfo,
        client: ContestManagerClient,
        model_name: str,
        contest_id: str,
        participant_id: str,
        problems: list = None
    ):
        try:
            self._update_agent_status_from_step(agent_info, "analyze_contest")
            logger.info(f"Starting agent {agent_info.agent_id} for model {model_name}")
            logger.info(f"Contest: {contest_id}, Participant: {participant_id}")
            
            logger.info("Creating agent from model...")
            
            def status_callback(step: str):
                self._update_agent_status_from_step(agent_info, step)
            
            agent = self.agent_manager.create_agent_from_model(
                model_name,
                client,
                contest_id,
                participant_id,
                agent_id=agent_info.agent_id,
                status_callback=status_callback,
                problems=problems
            )
            logger.info(f"Agent created successfully: {type(agent).__name__}")
            try:
                monitor_task = asyncio.create_task(self._monitor_agent_status(agent_info, agent))
                logger.info(f"Created monitoring task for agent {agent_info.agent_id}")
            except Exception as e:
                logger.error(f"Failed to create monitoring task: {e}")
                agent_info.status = agent_manager_pb2.AGENT_STATUS_FAILED
                agent_info.error_message = str(e)
                agent_info.completed_at = datetime.now()
                self._broadcast_status_update(agent_info)
            
            logger.info("Starting agent participation in contest...")
            result = await agent.participate(contest_id, participant_id)
            logger.info(f"Agent participation completed with result: {result}")
            
            final_state = None
            if hasattr(agent, '_current_state') and agent._current_state:
                final_state = agent._current_state.get('current_step')
            
            if final_state == "contest_ended":
                agent_info.status = agent_manager_pb2.AGENT_STATUS_CONTEST_ENDED
                logger.info(f"Agent {agent_info.agent_id} ended due to contest completion")
            else:
                agent_info.status = agent_manager_pb2.AGENT_STATUS_COMPLETED
                logger.info(f"Agent {agent_info.agent_id} completed successfully")
            
            agent_info.completed_at = datetime.now()
            self._broadcast_status_update(agent_info)
            
            logger.info(f"Agent {agent_info.agent_id} completed successfully")
            
        except Exception as e:
            import traceback
            logger.error(f"Agent {agent_info.agent_id} failed: {e}")
            logger.error(f"Traceback:\n{traceback.format_exc()}")
            agent_info.status = agent_manager_pb2.AGENT_STATUS_FAILED
            agent_info.error_message = str(e)
            agent_info.completed_at = datetime.now()
            self._broadcast_status_update(agent_info)
    
    def CreateAgent(self, request, context):
        try:
            agent_id = str(uuid.uuid4())
            
            logger.info(
                f"Creating agent {agent_id} for contest {request.contest_id}, "
                f"participant {request.participant_id}, model {request.model_name}, "
                f"problems: {len(request.problems)}"
            )
            
            # Convert protobuf problems to a format the agent can use
            problems = []
            for pb_problem in request.problems:
                problems.append({
                    'id': pb_problem.id,
                    'name': pb_problem.name,
                    'description': pb_problem.description,
                    'time_limit_ms': pb_problem.time_limit_ms,
                    'memory_limit_mb': pb_problem.memory_limit_mb
                })
            
            host = request.contest_manager_host or "localhost:50051"
            client = ContestManagerClient(host)
            client.connect()
            
            agent_info = AgentInfo(
                agent_id=agent_id,
                contest_id=request.contest_id,
                participant_id=request.participant_id,
                model_name=request.model_name,
                task=None
            )
            
            task = asyncio.run_coroutine_threadsafe(
                self._run_agent_async(
                    agent_info,
                    client,
                    request.model_name,
                    request.contest_id,
                    request.participant_id,
                    problems
                ),
                self.event_loop
            )
            agent_info.task = task
            
            self.agents[agent_id] = agent_info
            
            self._broadcast_status_update(agent_info)
            
            logger.info(f"Agent {agent_id} created and started")
            
            return agent_manager_pb2.CreateAgentResponse(
                agent_id=agent_id,
                contest_id=request.contest_id,
                participant_id=request.participant_id,
                model_name=request.model_name,
                status=agent_manager_pb2.AGENT_STATUS_ANALYZING_CONTEST,
                message=f"Agent {agent_id} created and started successfully"
            )
            
        except Exception as e:
            logger.error(f"Failed to create agent: {e}")
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(f"Failed to create agent: {str(e)}")
            return agent_manager_pb2.CreateAgentResponse()
    
    def GetAgentStatus(self, request, context):
        agent_id = request.agent_id
        
        if agent_id not in self.agents:
            context.set_code(grpc.StatusCode.NOT_FOUND)
            context.set_details(f"Agent {agent_id} not found")
            return agent_manager_pb2.AgentStatusResponse()
        
        agent_info = self.agents[agent_id]
        
        return agent_manager_pb2.AgentStatusResponse(
            agent_id=agent_info.agent_id,
            contest_id=agent_info.contest_id,
            participant_id=agent_info.participant_id,
            model_name=agent_info.model_name,
            status=agent_info.status
        )
    
    def ListAgents(self, request, context):
        agents = []
        
        for agent_id, agent_info in self.agents.items():
            if request.HasField("contest_id") and agent_info.contest_id != request.contest_id:
                continue
            if request.HasField("status") and agent_info.status != request.status:
                continue
            
            agents.append(
                agent_manager_pb2.AgentStatusResponse(
                    agent_id=agent_info.agent_id,
                    contest_id=agent_info.contest_id,
                    participant_id=agent_info.participant_id,
                    model_name=agent_info.model_name,
                    status=agent_info.status
                )
            )
        
        return agent_manager_pb2.ListAgentsResponse(agents=agents)
    
    def StopAgent(self, request, context):
        agent_id = request.agent_id
        
        if agent_id not in self.agents:
            context.set_code(grpc.StatusCode.NOT_FOUND)
            context.set_details(f"Agent {agent_id} not found")
            return agent_manager_pb2.StopAgentResponse()
        
        agent_info = self.agents[agent_id]
        
        try:
            agent_info.task.cancel()
            agent_info.status = agent_manager_pb2.AGENT_STATUS_STOPPED
            agent_info.completed_at = datetime.now()
            
            self._broadcast_status_update(agent_info)
            
            logger.info(f"Agent {agent_id} stopped: {request.reason}")
            
            return agent_manager_pb2.StopAgentResponse(
                agent_id=agent_id,
                success=True,
                message=f"Agent {agent_id} stopped successfully"
            )
            
        except Exception as e:
            logger.error(f"Failed to stop agent {agent_id}: {e}")
            return agent_manager_pb2.StopAgentResponse(
                agent_id=agent_id,
                success=False,
                message=f"Failed to stop agent: {str(e)}"
            )
    
    def _broadcast_status_update(self, agent_info: AgentInfo):
        update = agent_manager_pb2.AgentStatusUpdate(
            agent_id=agent_info.agent_id,
            contest_id=agent_info.contest_id,
            participant_id=agent_info.participant_id,
            model_name=agent_info.model_name,
            status=agent_info.status,
            timestamp=int(time.time())
        )
        
        for queue in self.status_subscribers:
            try:
                queue.put_nowait(update)
            except asyncio.QueueFull:
                logger.warning("Status update queue full, dropping update")
    
    def StreamAgentStatus(self, request, context):
        logger.info(f"Starting status stream for contest: {request.contest_id if request.HasField('contest_id') else 'all'}")
        
        queue = asyncio.Queue(maxsize=100)
        self.status_subscribers.append(queue)
        
        try:
            for agent_id, agent_info in self.agents.items():
                if request.HasField("contest_id") and agent_info.contest_id != request.contest_id:
                    continue
                
                yield agent_manager_pb2.AgentStatusUpdate(
                    agent_id=agent_info.agent_id,
                    contest_id=agent_info.contest_id,
                    participant_id=agent_info.participant_id,
                    model_name=agent_info.model_name,
                    status=agent_info.status,
                    timestamp=int(time.time())
                )
            
            while context.is_active():
                try:
                    future = asyncio.run_coroutine_threadsafe(
                        asyncio.wait_for(queue.get(), timeout=1.0),
                        self.event_loop
                    )
                    
                    try:
                        update = future.result(timeout=1.5)
                        
                        if request.HasField("contest_id") and update.contest_id != request.contest_id:
                            continue
                        
                        yield update
                    except TimeoutError:
                        continue
                        
                except Exception as e:
                    logger.error(f"Error in status stream: {e}")
                    break
                    
        finally:
            if queue in self.status_subscribers:
                self.status_subscribers.remove(queue)
            logger.info("Status stream closed")


class AgentManagerServer:
    def __init__(self, port: int = 50052, max_workers: int = 10):
        self.port = port
        self.max_workers = max_workers
        self.server = None
        logger.info(f"AgentManagerServer initialized on port {port}")
    
    def start(self):
        self.server = grpc.server(
            futures.ThreadPoolExecutor(max_workers=self.max_workers)
        )
        
        agent_manager_pb2_grpc.add_AgentManagerServiceServicer_to_server(
            AgentManagerServicer(), self.server
        )
        
        self.server.add_insecure_port(f"[::]:{self.port}")
        self.server.start()
        
        logger.info(f"AgentManager gRPC server started on port {self.port}")
    
    def stop(self, grace_period: int = 5):
        if self.server:
            logger.info("Stopping AgentManager gRPC server...")
            self.server.stop(grace_period)
            logger.info("AgentManager gRPC server stopped")
    
    def wait_for_termination(self):
        if self.server:
            self.server.wait_for_termination()

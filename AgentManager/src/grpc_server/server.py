import asyncio
import logging
import uuid
import sys
import os
from concurrent import futures
from datetime import datetime
from typing import Dict, Optional
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
        self.problems_solved = 0
        self.problems_attempted = 0
        self.current_problem = ""
        self.started_at = datetime.now()
        self.completed_at: Optional[datetime] = None
        self.error_message = ""


class AgentManagerServicer(agent_manager_pb2_grpc.AgentManagerServiceServicer):
    
    def __init__(self):
        self.agent_manager = AgentManager()
        self.agents: Dict[str, AgentInfo] = {}
        # Create a new event loop for async tasks
        self.event_loop = asyncio.new_event_loop()
        # Start the event loop in a separate thread
        import threading
        self.loop_thread = threading.Thread(target=self._run_event_loop, daemon=True)
        self.loop_thread.start()
        logger.info("AgentManagerServicer initialized")
    
    def _run_event_loop(self):
        """Run the event loop in a separate thread."""
        asyncio.set_event_loop(self.event_loop)
        self.event_loop.run_forever()
    
    async def _run_agent_async(
        self,
        agent_info: AgentInfo,
        client: ContestManagerClient,
        model_name: str,
        contest_id: str,
        participant_id: str
    ):
        try:
            agent_info.status = agent_manager_pb2.AGENT_STATUS_RUNNING
            logger.info(f"Starting agent {agent_info.agent_id} for model {model_name}")
            logger.info(f"Contest: {contest_id}, Participant: {participant_id}")
            
            logger.info("Creating agent from model...")
            agent = self.agent_manager.create_agent_from_model(
                model_name,
                client,
                contest_id,
                participant_id
            )
            logger.info(f"Agent created successfully: {type(agent).__name__}")
            
            logger.info("Starting agent participation in contest...")
            result = await agent.participate(contest_id, participant_id)
            logger.info(f"Agent participation completed with result: {result}")
            
            agent_info.status = agent_manager_pb2.AGENT_STATUS_COMPLETED
            agent_info.completed_at = datetime.now()
            
            logger.info(f"Agent {agent_info.agent_id} completed successfully")
            
        except Exception as e:
            import traceback
            logger.error(f"Agent {agent_info.agent_id} failed: {e}")
            logger.error(f"Traceback:\n{traceback.format_exc()}")
            agent_info.status = agent_manager_pb2.AGENT_STATUS_FAILED
            agent_info.error_message = str(e)
            agent_info.completed_at = datetime.now()
    
    def CreateAgent(self, request, context):
        try:
            agent_id = str(uuid.uuid4())
            
            logger.info(
                f"Creating agent {agent_id} for contest {request.contest_id}, "
                f"participant {request.participant_id}, model {request.model_name}"
            )
            
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
                    request.participant_id
                ),
                self.event_loop
            )
            agent_info.task = task
            
            self.agents[agent_id] = agent_info
            
            logger.info(f"Agent {agent_id} created and started")
            
            return agent_manager_pb2.CreateAgentResponse(
                agent_id=agent_id,
                contest_id=request.contest_id,
                participant_id=request.participant_id,
                model_name=request.model_name,
                status=agent_manager_pb2.AGENT_STATUS_RUNNING,
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
            status=agent_info.status,
            problems_solved=agent_info.problems_solved,
            problems_attempted=agent_info.problems_attempted,
            current_problem=agent_info.current_problem,
            started_at=int(agent_info.started_at.timestamp()),
            completed_at=int(agent_info.completed_at.timestamp()) if agent_info.completed_at else 0,
            error_message=agent_info.error_message
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
                    status=agent_info.status,
                    problems_solved=agent_info.problems_solved,
                    problems_attempted=agent_info.problems_attempted,
                    current_problem=agent_info.current_problem,
                    started_at=int(agent_info.started_at.timestamp()),
                    completed_at=int(agent_info.completed_at.timestamp()) if agent_info.completed_at else 0,
                    error_message=agent_info.error_message
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

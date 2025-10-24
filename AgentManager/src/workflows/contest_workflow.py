import logging
from langchain_core.language_models import BaseChatModel
from langchain_core.messages import HumanMessage

from .state import AgentState, StateManager
from .steps import WorkflowSteps
from .config import WorkflowConfig
from ..models import AgentConfig, Problem
from ..grpc_client import ContestManagerClient
from ..tools import create_contest_tools
from ..config.langsmith_config import create_contest_tracer, log_contest_event
from ..config import get_settings


class ContestAgent:

    def __init__(self, config: AgentConfig, client: ContestManagerClient, llm: BaseChatModel, 
                 contest_id: str = None, participant_id: str = None, agent_id: str = None, 
                 status_callback=None, problems: list = None):
        self.config = config
        self.client = client
        self.llm = llm
        self.contest_id = contest_id
        self.participant_id = participant_id
        self.agent_id = agent_id or "unknown"
        
        self.problems = []
        if problems:
            for p in problems:
                if isinstance(p, dict):
                    self.problems.append(Problem(
                        id=p.get("id", ""),
                        name=p.get("name", ""),
                        description=p.get("description", ""),
                        time_limit_ms=p.get("time_limit_ms", 1000),
                        memory_limit_mb=p.get("memory_limit_mb", 256),
                        tag=p.get("tag", "")
                    ))
                else:
                    self.problems.append(p)
        
        self.logger = logging.getLogger(__name__)
        
        self.state_manager = StateManager(status_callback)
        
        self.tracer = create_contest_tracer(contest_id or "unknown", participant_id or "unknown", agent_id)
        
        self.contest_tools = create_contest_tools(client)
        self.all_tools = self.contest_tools
        
        callbacks = [self.tracer] if self.tracer else []
        self.llm_with_tools = llm.bind_tools(self.all_tools)
        
        self.workflow_steps = WorkflowSteps(client, self.contest_tools, self.llm_with_tools, self.state_manager, config.model_name)
        self.workflow_config = WorkflowConfig(client)
        workflow_graph = self.workflow_config.create_workflow_graph(self.workflow_steps)
        self.workflow = workflow_graph.compile()
        
        if contest_id and participant_id:
            log_contest_event(
                "agent_created", 
                contest_id, 
                participant_id,
                data={"model": config.model_name, "provider": config.model_provider}
            )
    
    @property
    def _current_state(self):
        """Get current workflow state for compatibility."""
        return self.state_manager.get_current_state()
    
    async def participate(self, contest_id: str, participant_id: str):
        self.logger.info(f"[{self.config.model_name}] Starting contest participation for contest {contest_id}")
        
        initial_state = AgentState(
            messages=[HumanMessage(content="Starting contest participation")],
            contest_id=contest_id,
            participant_id=participant_id,
            current_problem=None,
            contest_info=None,
            remaining_time=0,
            submitted_problems=[],
            solved_problems=[], 
            current_step="start",
            error_message=None,
            extracted_code=None,
            solution_language=None,
            available_problems=self.problems
        )
        
        try:
            settings = get_settings()
            config = {
                "recursion_limit": settings.workflow_recursion_limit
            }
            if self.tracer:
                config["callbacks"] = [self.tracer]
            
            final_state = await self.workflow.ainvoke(initial_state, config=config)
            self.logger.info(f"[{self.config.model_name}] Contest participation completed. Final step: {final_state['current_step']}")
            
            return final_state
            
        except Exception as e:
            self.logger.error(f"[{self.config.model_name}] Error during contest participation: {e}")
            raise

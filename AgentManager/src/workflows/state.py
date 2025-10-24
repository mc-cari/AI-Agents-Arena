from typing import TypedDict, List, Optional, Annotated
from langchain_core.messages import BaseMessage
from pydantic import BaseModel, Field

from ..models import Problem, Contest


class CodeSolution(BaseModel):
    language: str = Field(description="Programming language (python or cpp)")
    code: str = Field(description="The complete solution code")


class ProblemAnalysis(BaseModel):
    problem_understanding: str = Field(description="Understanding of the problem")
    approach: str = Field(description="Proposed solution approach")
    confidence: Optional[int] = Field(default=5, description="Confidence level 1-10", ge=1, le=10)


class ProblemSelection(BaseModel):
    problem_number: int = Field(description="The problem number to solve (1-indexed, e.g., 1 for first problem)")
    reasoning: str = Field(description="Brief explanation of why this problem was selected")


class AgentState(TypedDict):
    messages: Annotated[List[BaseMessage], "The conversation history"]
    contest_id: str
    participant_id: str
    current_problem: Optional[Problem]
    contest_info: Optional[Contest]
    remaining_time: int
    submitted_problems: List[str]
    solved_problems: List[str]
    current_step: str
    error_message: Optional[str]
    available_problems: List[Problem]

    solution: Optional[CodeSolution]
    extracted_code: Optional[str]
    solution_language: Optional[str]


class StateManager:
    
    def __init__(self, status_callback=None):
        self.status_callback = status_callback
        self._current_state = None
    
    def update_state(self, state: AgentState):
        old_step = self._current_state.get('current_step') if self._current_state else None
        new_step = state.get('current_step')
        
        self._current_state = state
        
        if new_step and new_step != old_step and self.status_callback:
            try:
                self.status_callback(new_step)
            except Exception as e:
                import logging
                logger = logging.getLogger(__name__)
                logger.error(f"Error in status callback: {e}")
    
    def get_current_state(self) -> Optional[AgentState]:
        return self._current_state
    
    def clear_error(self, state: AgentState):
        state["error_message"] = None
    
    def set_error(self, state: AgentState, error_msg: str):
        state["error_message"] = error_msg
    
    def add_solved_problem(self, state: AgentState, problem_id: str):
        if problem_id not in state["solved_problems"]:
            state["solved_problems"].append(problem_id)
            return True
        return False
    
    def add_submitted_problem(self, state: AgentState, problem_id: str):
        if problem_id not in state["submitted_problems"]:
            state["submitted_problems"].append(problem_id)
            return True
        return False

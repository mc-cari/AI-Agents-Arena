from datetime import datetime
from langgraph.graph import StateGraph, END

from .state import AgentState


class WorkflowConfig:
    
    def __init__(self, client):
        self.client = client
    
    def should_continue(self, state: AgentState) -> str:
        if state.get("current_step") == "contest_ended":
            return "end"
        
        if state["remaining_time"] <= 20:
            return "end"
        
        available_problems = state.get("available_problems", [])
        if len(state["solved_problems"]) >= len(available_problems):
            return "end"
        
        if state["error_message"] and state.get("current_step") != "contest_ended":
            return "select_new"

        return "continue"
    
    def check_time_remaining(self, state: AgentState) -> str:
        try:
            fresh_contest = self.client.get_contest(state["contest_id"])
            if not fresh_contest:
                return "end"
            
            now = datetime.now()
            remaining = (fresh_contest.ends_at - now).total_seconds()
            state["remaining_time"] = max(0, int(remaining))
            
            if remaining <= 30:
                return "end"
            
            return "select_problem"
            
        except Exception:
            return "end"
    
    def create_workflow_graph(self, workflow_steps) -> StateGraph:
        workflow = StateGraph(AgentState)
        
        workflow.add_node("analyze_contest", workflow_steps.analyze_contest)
        workflow.add_node("select_problem", workflow_steps.select_problem)
        workflow.add_node("solve_problem", workflow_steps.solve_problem)
        workflow.add_node("submit_solution", workflow_steps.submit_solution)
        workflow.add_node("check_results", workflow_steps.check_results)
        
        workflow.set_entry_point("analyze_contest")
        
        workflow.add_edge("analyze_contest", "select_problem")
        
        workflow.add_conditional_edges(
            "select_problem",
            lambda state: "end" if state.get("current_step") == "no_problems" else "solve_problem",
            {
                "solve_problem": "solve_problem",
                "end": END
            }
        )
        
        workflow.add_edge("solve_problem", "submit_solution")
        
        workflow.add_conditional_edges(
            "submit_solution",
            lambda state: "end" if state.get("current_step") == "contest_ended" else "check_results",
            {
                "check_results": "check_results",
                "end": END
            }
        )
        
        workflow.add_conditional_edges(
            "check_results",
            self.check_time_remaining,
            {
                "select_problem": "select_problem",
                "end": END
            }
        )
        
        return workflow

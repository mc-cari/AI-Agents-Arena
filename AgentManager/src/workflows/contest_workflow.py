from typing import TypedDict, List, Optional, Annotated
from langgraph.graph import StateGraph, END
from langchain_core.messages import BaseMessage, HumanMessage, AIMessage
from langchain_core.language_models import BaseChatModel
from datetime import datetime, timedelta
import logging
import asyncio
from pydantic import BaseModel, Field
from openai import RateLimitError as OpenAIRateLimitError
from anthropic import RateLimitError as AnthropicRateLimitError
try:
    from google.api_core.exceptions import ResourceExhausted as GoogleRateLimitError
except ImportError:
    GoogleRateLimitError = None

from ..models import Contest, Problem, AgentConfig
from ..grpc_client import ContestManagerClient
from ..tools import create_contest_tools
from ..config.langsmith_config import create_contest_tracer, log_contest_event


class CodeSolution(BaseModel):
    language: str = Field(description="Programming language (python or cpp)")
    code: str = Field(description="The complete solution code")


class ProblemAnalysis(BaseModel):
    problem_understanding: str = Field(description="Understanding of the problem")
    approach: str = Field(description="Proposed solution approach")
    edge_cases: List[str] = Field(description="Important edge cases to consider")
    confidence: int = Field(description="Confidence level 1-10", ge=1, le=10)


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

    solution: Optional[CodeSolution]
    extracted_code: Optional[str]
    solution_language: Optional[str]


class ContestAgent:

    def __init__(self, config: AgentConfig, client: ContestManagerClient, llm: BaseChatModel, 
                 contest_id: str = None, participant_id: str = None, agent_id: str = None):
        self.config = config
        self.client = client
        self.llm = llm
        self.logger = logging.getLogger(__name__)
        self.contest_id = contest_id
        self.participant_id = participant_id
        self.agent_id = agent_id or "unknown"
        
        self.tracer = create_contest_tracer(contest_id or "unknown", participant_id or "unknown", agent_id)
        
        self.contest_tools = create_contest_tools(client)
        self.all_tools = self.contest_tools
        
        callbacks = [self.tracer] if self.tracer else []
        self.llm_with_tools = llm.bind_tools(self.all_tools)
        
        self.workflow = self._create_workflow()
        
        if contest_id and participant_id:
            log_contest_event(
                "agent_created", 
                contest_id, 
                participant_id,
                data={"model": config.model_name, "provider": config.model_provider}
            )
    
    def _handle_contest_end_error(self, state: AgentState, error_msg: str, context: str = "operation") -> bool:
        """
        Check if error indicates contest has ended and update state accordingly.
        
        Args:
            state: Current agent state
            error_msg: Error message to check
            context: Context where error occurred (e.g., "submission", "check_results")
            
        Returns:
            True if contest ended, False otherwise
        """
        contest_end_indicators = [
            "contest is not accepting submissions",
            "contest has ended",
            "contest is finished",
            "contest not found"
        ]
        
        if any(indicator in error_msg.lower() for indicator in contest_end_indicators):
            self.logger.warning(f"Contest has ended during {context}, stopping agent")
            state["current_step"] = "contest_ended"
            state["error_message"] = "Contest ended"
            
            log_contest_event(
                f"contest_ended_during_{context}",
                state["contest_id"],
                state["participant_id"],
                data={
                    "submitted_problems": len(state["submitted_problems"]),
                    "solved_problems": len(state.get("solved_problems", [])),
                    "attempted_problem": state["current_problem"].id if state.get("current_problem") else None,
                    "error_message": error_msg
                }
            )
            return True
        
        return False
    
    async def _invoke_llm_with_backoff(self, llm_instance, messages, max_retries=5):
        base_delay = 1  
        max_delay = 60 
        
        rate_limit_exceptions = (OpenAIRateLimitError, AnthropicRateLimitError)
        if GoogleRateLimitError is not None:
            rate_limit_exceptions = rate_limit_exceptions + (GoogleRateLimitError,)
        
        for attempt in range(max_retries):
            try:
                response = await llm_instance.ainvoke(messages)
                
                if hasattr(response, 'response_metadata') and 'token_usage' in response.response_metadata:
                    usage = response.response_metadata['token_usage']
                    self.logger.debug(f"Token usage - Prompt: {usage.get('prompt_tokens', 0)}, "
                                    f"Completion: {usage.get('completion_tokens', 0)}, "
                                    f"Total: {usage.get('total_tokens', 0)}")
                
                return response
                
            except rate_limit_exceptions as e:
                if attempt == max_retries - 1:
                    self.logger.error(f"Rate limit error after {max_retries} attempts: {e}")
                    raise
                
                delay = min(base_delay * (2 ** attempt), max_delay)
                
                provider = "unknown"
                if isinstance(e, OpenAIRateLimitError):
                    provider = "OpenAI"
                elif isinstance(e, AnthropicRateLimitError):
                    provider = "Anthropic"
                elif GoogleRateLimitError and isinstance(e, GoogleRateLimitError):
                    provider = "Google Gemini"
                
                self.logger.warning(f"{provider} rate limit hit (attempt {attempt + 1}/{max_retries}). "
                                  f"Retrying in {delay:.1f} seconds...")
                
                log_contest_event(
                    "rate_limit_hit",
                    self.contest_id,
                    self.participant_id,
                    data={
                        "attempt": attempt + 1,
                        "delay": delay,
                        "model": self.config.model_name,
                        "provider": provider
                    }
                )
                
                await asyncio.sleep(delay)
            except Exception as e:
                self.logger.error(f"LLM invocation error: {e}")
                raise
    
    def _create_workflow(self) -> StateGraph:
        workflow = StateGraph(AgentState)
        
        workflow.add_node("analyze_contest", self._analyze_contest)
        workflow.add_node("select_problem", self._select_problem)
        workflow.add_node("solve_problem", self._solve_problem)
        workflow.add_node("submit_solution", self._submit_solution)
        workflow.add_node("check_results", self._check_results)
        workflow.add_node("monitor_contest", self._monitor_contest)
        
        workflow.add_edge("analyze_contest", "select_problem")
        
        workflow.add_conditional_edges(
            "select_problem",
            lambda state: "end" if state.get("error_message") or state.get("current_problem") is None else "solve_problem",
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
            self._should_continue,
            {
                "continue": "monitor_contest",
                "select_new": "select_problem",
                "end": END
            }
        )
        
        workflow.add_conditional_edges(
            "monitor_contest",
            self._check_time_remaining,
            {
                "continue": "select_problem",
                "end": END
            }
        )
        
        workflow.set_entry_point("analyze_contest")
        
        return workflow.compile()
    
    async def _analyze_contest(self, state: AgentState) -> AgentState:
        self.logger.info(f"Analyzing contest {state['contest_id']}")
        
        try:
            view_contest_tool = next(t for t in self.contest_tools if t.name == "view_contest")
            contest_info = view_contest_tool.run({"contest_id": state["contest_id"]})
            
            contest = self.client.get_contest(state["contest_id"])
            if not contest:
                raise ValueError(f"Contest {state['contest_id']} not found")

            state["contest_info"] = contest
            state["current_step"] = "analyze_contest"
            
            now = datetime.now()
            remaining = (contest.ends_at - now).total_seconds()
            state["remaining_time"] = max(0, int(remaining))
            
            log_contest_event(
                "contest_analyzed",
                state["contest_id"],
                state["participant_id"],
                data={
                    "remaining_time": state["remaining_time"],
                    "num_problems": len(contest.problems),
                    "num_participants": len(contest.participants)
                }
            )
            
            analysis_prompt = f"""
            Contest Analysis (using tools):
            {contest_info}
            
            Time remaining: {state['remaining_time']} seconds
            
            As a competitive programming agent, your goal is to solve as many problems as possible
            within the time limit. Use the available tools to analyze problems and make strategic decisions.
            """
            
            state["messages"].append(HumanMessage(content=analysis_prompt))
            
        except Exception as e:
            self.logger.error(f"Error analyzing contest: {e}")
            state["error_message"] = str(e)
        
        return state
    
    async def _select_problem(self, state: AgentState) -> AgentState:
        self.logger.info("Selecting problem to solve")
        
        try:
            contest = state["contest_info"]
            solved = state["solved_problems"]

            available_problems = [p for p in contest.problems if p.id not in solved]
            
            if not available_problems:
                state["current_step"] = "no_problems"
                self.logger.info("All problems have been solved!")
                return state
            
            select_tool = next(t for t in self.contest_tools if t.name == "select_problem")
            selection_prompt = select_tool.run({
                "contest_id": state["contest_id"],
                "solved_problems": solved,
                "time_remaining": state["remaining_time"]
            })
            
            full_prompt = f"""
            You are participating in a competitive programming contest. You need to select which problem to solve next.

            {selection_prompt}

            As an expert competitive programmer, analyze these problems and select the best one to solve next. Consider:
            1. Problem difficulty (based on description and constraints)
            2. Time remaining in the contest
            3. Your current progress
            4. Problem types that might be easier/faster to implement
            """
            
            state["messages"].append(HumanMessage(content=full_prompt))
            
            response = await self._invoke_llm_with_backoff(self.llm, state["messages"])
            state["messages"].append(response)
            
            response_text = response.content.strip()
            lines = response_text.split('\n')
            
            try:
                problem_number = int(lines[0].strip())
                if 1 <= problem_number <= len(available_problems):
                    selected_problem = available_problems[problem_number - 1]
                    reasoning = lines[1] if len(lines) > 1 else "No reasoning provided"
                else:
                    selected_problem = available_problems[0]
                    reasoning = "Invalid selection, using first available problem"
                    self.logger.warning(f"LLM selected invalid problem number: {problem_number}")
            except (ValueError, IndexError):
                selected_problem = available_problems[0]
                reasoning = "Failed to parse LLM selection, using first available problem"
                self.logger.warning(f"Failed to parse LLM selection: {response_text}")
            
            state["current_problem"] = selected_problem
            state["current_step"] = "select_problem"
            
            view_problem_tool = next(t for t in self.contest_tools if t.name == "view_problem")
            problem_details = view_problem_tool.run({
                "contest_id": state["contest_id"],
                "problem_id": selected_problem.id
            })
            
            problem_prompt = f"""
            Selected Problem: {selected_problem.name} (ID: {selected_problem.id})
            Selection Reasoning: {reasoning}

            {problem_details}

            Now analyze this problem thoroughly and prepare to solve it step by step.
            """
            
            state["messages"].append(HumanMessage(content=problem_prompt))
            self.logger.info(f"LLM selected problem: {selected_problem.name} - {reasoning}")
            
        except Exception as e:
            self.logger.error(f"Error selecting problem: {e}")
            state["error_message"] = str(e)
        
        return state
    
    async def _solve_problem(self, state: AgentState) -> AgentState:
        if state.get("current_problem") is None:
            self.logger.error("Cannot solve problem: current_problem is None")
            state["error_message"] = "No problem selected"
            state["current_step"] = "contest_ended"
            return state
        
        self.logger.info(f"Solving problem {state['current_problem'].id}")
        
        try:
            problem = state["current_problem"]
            
            analysis_prompt = f"""
            Analyze this competitive programming problem and provide a structured analysis:
            
            Problem: {problem.name}
            Description: {problem.description}
            
            Provide your analysis in the following structured format:
            - problem_understanding: Your understanding of what the problem is asking
            - approach: Your proposed solution approach/algorithm
            - edge_cases: List of important edge cases to consider
            - confidence: Your confidence level (1-10) in solving this problem
            """
            
            analysis_llm = self.llm.with_structured_output(ProblemAnalysis)
            analysis_response = await self._invoke_llm_with_backoff(analysis_llm, [HumanMessage(content=analysis_prompt)])
            
            self.logger.info(f"Problem analysis - Confidence: {analysis_response.confidence}/10")
            self.logger.info(f"Approach: {analysis_response.approach}")
            
            # Now solve the problem using structured output
            solve_prompt = f"""
            Based on your analysis, provide a complete solution to this problem:
            
            Problem: {problem.name}
            Description: {problem.description}
            
            Your analysis:
            - Understanding: {analysis_response.problem_understanding}
            - Approach: {analysis_response.approach}
            - Edge cases: {', '.join(analysis_response.edge_cases)}
            
            Provide your solution in the following structured format:
            - language: Choose either "python" or "cpp" (prefer cpp for competitive programming)
            - code: The complete, runnable solution code
            
            The code must be a complete, working program that:
            1. Reads input correctly
            2. Implements the algorithm to solve the problem
            3. Outputs the result in the expected format
            4. Handles all edge cases
            """
            
            solution_llm = self.llm.with_structured_output(CodeSolution)
            solution_response = await self._invoke_llm_with_backoff(solution_llm, [HumanMessage(content=solve_prompt)])
            
            self.logger.info(f"Generated {solution_response.language} solution")
            
            log_contest_event(
                "problem_solved",
                state["contest_id"],
                state["participant_id"],
                data={
                    "problem_id": problem.id,
                    "problem_name": problem.name,
                    "language": solution_response.language,
                    "code_length": len(solution_response.code),
                    "confidence": analysis_response.confidence
                }
            )
            
            state["solution"] = solution_response
            state["extracted_code"] = solution_response.code
            state["solution_language"] = solution_response.language
            
            state["current_step"] = "solve_problem"
            
        except Exception as e:
            self.logger.error(f"Error solving problem: {e}")
            state["error_message"] = str(e)
        
        return state
    
    async def _submit_solution(self, state: AgentState) -> AgentState:
        self.logger.info("Submitting solution")
        
        try:
            if "solution" in state and state["solution"]:
                solution = state["solution"]
                code = solution.code
                language = solution.language
                
                self.logger.info(f"Using structured solution - Language: {language}")
                self.logger.info(f"Code length: {len(code)} characters")
                
            elif "extracted_code" in state and state["extracted_code"]:
                code = state["extracted_code"]
                language = state.get("solution_language", "cpp") 
                
                self.logger.info(f"Using extracted code - Language: {language}")
                self.logger.info(f"Code length: {len(code)} characters")
            
            if code:
                submit_tool = next(t for t in self.contest_tools if t.name == "submit_solution")
                result = submit_tool.run({
                    "contest_id": state["contest_id"],
                    "participant_id": state["participant_id"],
                    "problem_id": state["current_problem"].id,
                    "code": code,
                    "language": language
                })
                
                if "successfully" in result.lower():
                    state["submitted_problems"].append(state["current_problem"].id)
                    state["messages"].append(AIMessage(content=result))
                    self.logger.info(result)
                    
                    log_contest_event(
                        "solution_submitted",
                        state["contest_id"],
                        state["participant_id"],
                        data={
                            "problem_id": state["current_problem"].id,
                            "problem_name": state["current_problem"].name,
                            "language": language,
                            "code_length": len(code)
                        }
                    )
                else:
                    raise Exception(f"Submit tool failed: {result}")
            
            state["current_step"] = "submit_solution"
            
        except Exception as e:
            error_msg = str(e)
            self.logger.error(f"Error submitting solution: {error_msg}")
            
            if not self._handle_contest_end_error(state, error_msg, "submission"):
                state["error_message"] = error_msg
        
        return state
    
    async def _check_results(self, state: AgentState) -> AgentState:
        self.logger.info("Checking submission results")
        
        try:
            await asyncio.sleep(2)
            
            self.logger.info(f"Looking for check_submission_results tool...")
            check_tool = next(t for t in self.contest_tools if t.name == "check_submission_results")
            self.logger.info(f"Found tool: {check_tool.name}")
            
            tool_args = {
                "contest_id": state["contest_id"],
                "participant_id": state["participant_id"],
                "problem_id": state["current_problem"].id
            }
            self.logger.info(f"Calling tool with args: {tool_args}")
            
            result_msg = check_tool.run(tool_args)
            self.logger.info(f"Tool returned: {result_msg}")
            
            if "error" in result_msg:
                raise ValueError(f"Error checking submission results: {result_msg}")

            if "🎉 PROBLEM SOLVED! ✅" in result_msg:
                problem_id = state["current_problem"].id
                if problem_id not in state["solved_problems"]:
                    state["solved_problems"].append(problem_id)
                    self.logger.info(f"Successfully solved problem: {state['current_problem'].name}")
            
            state["messages"].append(AIMessage(content=result_msg))
            self.logger.info(result_msg)
            state["current_step"] = "check_results"
            
        except Exception as e:
            error_msg = str(e)
            self.logger.error(f"Error checking results: {error_msg}")
            
            if not self._handle_contest_end_error(state, error_msg, "check_results"):
                state["error_message"] = error_msg
        
        return state
    
    async def _monitor_contest(self, state: AgentState) -> AgentState:
        self.logger.info("Monitoring contest")
        
        try:
            leaderboard_tool = next(t for t in self.contest_tools if t.name == "view_leaderboard")
            leaderboard = leaderboard_tool.run({"contest_id": state["contest_id"]})
            
            participants = self.client.get_leaderboard(state["contest_id"])
            our_participant = next(
                (p for p in participants if p.id == state["participant_id"]), 
                None
            )
            
            if our_participant:
                monitor_msg = f"Current rank: {our_participant.result.rank}, "
                monitor_msg += f"Solved: {our_participant.result.solved}\n\n"
                monitor_msg += f"Leaderboard:\n{leaderboard}"
                state["messages"].append(AIMessage(content=monitor_msg))
                self.logger.info(f"Current rank: {our_participant.result.rank}, Solved: {our_participant.result.solved}")
            else:
                state["messages"].append(AIMessage(content=f"Leaderboard:\n{leaderboard}"))
            
            progress_msg = f"Our progress: {len(state['solved_problems'])} solved, {len(state['submitted_problems'])} submitted"
            state["messages"].append(AIMessage(content=progress_msg))
            self.logger.info(progress_msg)
            
            state["current_step"] = "monitor_contest"
            
        except Exception as e:
            self.logger.error(f"Error monitoring contest: {e}")
            state["error_message"] = str(e)
        
        return state
    
    def _should_continue(self, state: AgentState) -> str:
        if state.get("current_step") == "contest_ended":
            self.logger.info("Contest ended, stopping agent")
            return "end"
        
        if state["remaining_time"] <= 20:
            self.logger.info("Time is running out, stopping agent")
            return "end"
        
        contest = state["contest_info"]
        if len(state["solved_problems"]) >= len(contest.problems):
            self.logger.info(f"All {len(contest.problems)} problems solved! Contest complete!")
            return "end"
        
        if state["error_message"] and state.get("current_step") != "contest_ended":
            return "select_new"

        return "continue"
    
    def _check_time_remaining(self, state: AgentState) -> str:
        try:
            fresh_contest = self.client.get_contest(state["contest_id"])
            if not fresh_contest:
                self.logger.warning("Contest not found, ending agent")
                return "end"
            
            if fresh_contest.state.value == "CONTEST_STATE_FINISHED":
                self.logger.info("Contest has finished, ending agent")
                
                log_contest_event(
                    "contest_ended",
                    state["contest_id"],
                    state["participant_id"],
                    data={
                        "submitted_problems": len(state["submitted_problems"]),
                        "solved_problems": len(state.get("solved_problems", [])),
                        "final_remaining_time": state["remaining_time"]
                    }
                )
                
                return "end"
            
            state["contest_info"] = fresh_contest
            contest = fresh_contest
        except Exception as e:
            self.logger.error(f"Error checking contest state: {e}")
            contest = state["contest_info"]
        
        now = datetime.now()
        remaining = (contest.ends_at - now).total_seconds()
        state["remaining_time"] = max(0, int(remaining))

        if state["remaining_time"] <= 20:
            self.logger.info("Time limit reached, ending agent")
            return "end"
        
        return "continue"
    
    async def _execute_tool(self, tool_call) -> str:
        tool_name = tool_call["name"]
        tool_args = tool_call["args"]
        

        tool = next((t for t in self.all_tools if t.name == tool_name), None)
        
        if tool:
            try:
                return tool.run(tool_args)
            except Exception as e:
                return f"Tool error: {e}"
        
        return f"Tool '{tool_name}' not found"
    
    async def participate(self, contest_id: str, participant_id: str) -> None:
        initial_state = AgentState(
            messages=[],
            contest_id=contest_id,
            participant_id=participant_id,
            current_problem=None,
            contest_info=None,
            remaining_time=0,
            submitted_problems=[],
            solved_problems=[], 
            current_step="start",
            error_message=None
        )
        
        try:
            # Configure LangSmith tracing with metadata and tags
            config = {
                "recursion_limit": 100,
                "metadata": {
                    "contest_id": contest_id,
                    "participant_id": participant_id,
                    "agent_id": self.agent_id,
                    "model_name": self.config.model_name if hasattr(self.config, 'model_name') else "unknown"
                },
                "tags": [
                    f"contest:{contest_id}",
                    f"participant:{participant_id}",
                    f"agent:{self.agent_id}"
                ]
            }
            
            # Add tracer to callbacks if available
            if self.tracer:
                config["callbacks"] = [self.tracer]
            
            final_state = await self.workflow.ainvoke(initial_state, config=config)
            self.logger.info(f"Contest participation completed. Final step: {final_state['current_step']}")
        except Exception as e:
            self.logger.error(f"Error during contest participation: {e}")
            raise

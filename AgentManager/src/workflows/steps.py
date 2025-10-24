import logging
from datetime import datetime
from langchain_core.messages import HumanMessage, AIMessage, ToolMessage, trim_messages, filter_messages
from openai import RateLimitError as OpenAIRateLimitError
from anthropic import RateLimitError as AnthropicRateLimitError
from tenacity import retry, stop_after_attempt, wait_exponential, retry_if_exception_type
from langsmith import traceable
from ..models import Problem
try:
    from google.api_core.exceptions import ResourceExhausted as GoogleRateLimitError
except ImportError:
    GoogleRateLimitError = None

from .state import AgentState, CodeSolution, ProblemAnalysis, ProblemSelection, StateManager
from ..config.langsmith_config import log_contest_event
from ..config import get_settings


class WorkflowSteps:
    
    def __init__(self, client, contest_tools, llm_with_tools, state_manager: StateManager, model_name: str = "unknown"):
        self.client = client
        self.contest_tools = contest_tools
        self.llm_with_tools = llm_with_tools
        self.state_manager = state_manager
        self.model_name = model_name
        self.logger = logging.getLogger(__name__)
        settings = get_settings()
        self.max_context_tokens = settings.max_context_tokens
    
    def _get_messages_with_context(self, state: AgentState, new_message: HumanMessage) -> list:
        if len(state["messages"]) == 0:
            return [new_message]
        
        try:
            filtered_messages = filter_messages(
                state["messages"],
                include_types=["human", "ai"], 
            )
            
            trimmed_messages = trim_messages(
                filtered_messages,
                max_tokens=self.max_context_tokens,
                strategy="last",  
                token_counter=self.llm_with_tools,  
            )
            
            context_msg_count = len(trimmed_messages)
            total_msg_count = len(state["messages"])
            
            if context_msg_count > 0:
                self.logger.debug(f"[{self.model_name}] Including {context_msg_count}/{total_msg_count} previous messages as context (max {self.max_context_tokens} tokens)")
            
            return trimmed_messages + [new_message]
        except Exception as e:
            self.logger.warning(f"[{self.model_name}] Error trimming messages: {e}, using last 3 messages as fallback")
            recent_messages = state["messages"][-3:] if len(state["messages"]) > 3 else state["messages"]
            return recent_messages + [new_message]
    
    @retry(
        stop=stop_after_attempt(3),
        wait=wait_exponential(multiplier=1, min=4, max=10),
        retry=retry_if_exception_type((OpenAIRateLimitError, AnthropicRateLimitError, TimeoutError)),
        reraise=True
    )
    async def _call_llm_with_retry(self, messages, structured_output=None):
        try:
            if structured_output:
                llm = self.llm_with_tools.with_structured_output(structured_output)
            else:
                llm = self.llm_with_tools
            
            response = await llm.ainvoke(messages)
            return response
                
        except (OpenAIRateLimitError, AnthropicRateLimitError) as e:
            self.logger.warning(f"[{self.model_name}] Rate limit hit, retrying...")
            raise
        except TimeoutError as e:
            self.logger.warning(f"[{self.model_name}] Request timed out, retrying...")
            raise
    
    async def _handle_tool_calls(self, messages: list, max_iterations: int = 5) -> tuple:
        response = await self._call_llm_with_retry(messages)
        iteration = 0
        
        while hasattr(response, 'tool_calls') and response.tool_calls and iteration < max_iterations:
            iteration += 1
            self.logger.info(f"[{self.model_name}] LLM is using {len(response.tool_calls)} tool(s) (iteration {iteration})")
            
            messages.append(response)
            
            for tool_call in response.tool_calls:
                tool_name = tool_call.get("name") if isinstance(tool_call, dict) else tool_call.name
                tool_args = tool_call.get("args", {}) if isinstance(tool_call, dict) else tool_call.args
                tool_call_id = tool_call.get("id", "") if isinstance(tool_call, dict) else getattr(tool_call, "id", "")
                
                self.logger.info(f"[{self.model_name}] Calling tool: {tool_name} with args: {tool_args}")
                
                tool = next((t for t in self.contest_tools if t.name == tool_name), None)
                if tool:
                    try:
                        tool_result = tool.run(tool_args)
                        self.logger.debug(f"[{self.model_name}] Tool {tool_name} result: {tool_result[:200]}...")
                        
                        tool_message = ToolMessage(
                            content=str(tool_result),
                            tool_call_id=tool_call_id,
                            name=tool_name
                        )
                        messages.append(tool_message)
                    except Exception as e:
                        self.logger.error(f"[{self.model_name}] Error executing tool {tool_name}: {e}")
                        tool_message = ToolMessage(
                            content=f"Error: {str(e)}",
                            tool_call_id=tool_call_id,
                            name=tool_name
                        )
                        messages.append(tool_message)
                else:
                    self.logger.warning(f"[{self.model_name}] Tool {tool_name} not found")
                    tool_message = ToolMessage(
                        content=f"Error: Tool {tool_name} not found",
                        tool_call_id=tool_call_id,
                        name=tool_name
                    )
                    messages.append(tool_message)
            
            response = await self._call_llm_with_retry(messages)
        
        if iteration >= max_iterations:
            self.logger.warning(f"[{self.model_name}] Reached max tool call iterations ({max_iterations})")
        
        return response, messages
    
    def _handle_contest_end_error(self, state: AgentState, error_msg: str, context: str = "operation") -> bool:
        contest_end_indicators = [
            "contest is not accepting submissions",
            "contest has ended",
            "contest is finished",
            "contest not found"
        ]
        
        if any(indicator in error_msg.lower() for indicator in contest_end_indicators):
            self.logger.warning(f"[{self.model_name}] Contest has ended during {context}, stopping agent")
            state["current_step"] = "contest_ended"
            state["error_message"] = "Contest ended"
            
            log_contest_event(
                "contest_ended",
                state["contest_id"],
                state["participant_id"],
                data={"context": context, "error": error_msg}
            )
            
            self.state_manager.update_state(state)
            return True
        
        return False
    
    @traceable(name="analyze_contest")
    async def analyze_contest(self, state: AgentState) -> AgentState:
        self.logger.info(f"[{self.model_name}] Analyzing contest")
        
        state["current_step"] = "analyze_contest"
        self.state_manager.update_state(state)
        
        try:
            view_contest_tool = next(t for t in self.contest_tools if t.name == "view_contest")
            contest_info = view_contest_tool.run({"contest_id": state["contest_id"]})
            
            contest = self.client.get_contest(state["contest_id"])
            if not contest:
                raise ValueError(f"Contest {state['contest_id']} not found")

            state["contest_info"] = contest
            
            now = datetime.now()
            remaining = (contest.ends_at - now).total_seconds()
            state["remaining_time"] = max(0, int(remaining))
            
            available_problems = state.get("available_problems", [])
            
            log_contest_event(
                "contest_analyzed",
                state["contest_id"],
                state["participant_id"],
                data={
                    "problems_count": len(available_problems),
                    "remaining_time": state["remaining_time"]
                }
            )
            
            analysis_msg = f"""
            Contest Analysis Complete:
            - Contest ID: {contest.id}
            - Problems: {len(available_problems)}
            - Time Remaining: {state['remaining_time']} seconds
            - Contest ends at: {contest.ends_at}
            """
            
            state["messages"].append(AIMessage(content=analysis_msg))
            self.logger.info(f"[{self.model_name}] {analysis_msg}")
            
        except Exception as e:
            self.logger.error(f"[{self.model_name}] Error analyzing contest: {e}")
            self.state_manager.set_error(state, str(e))
        return state
    @traceable(name="select_problem")
    async def select_problem(self, state: AgentState) -> AgentState:
        self.logger.info(f"[{self.model_name}] Selecting problem to solve")
        
        state["current_step"] = "select_problem"
        self.state_manager.update_state(state)
        
        try:
            all_problems = state.get("available_problems", [])
            
            self.logger.info(f"[{self.model_name}] Total problems available: {len(all_problems)}")
            self.logger.info(f"[{self.model_name}] Problems already solved: {len(state['solved_problems'])} - {state['solved_problems']}")
            self.logger.info(f"[{self.model_name}] Problems already submitted: {len(state['submitted_problems'])} - {state['submitted_problems']}")
            

            self.logger.info(f"[{self.model_name}] Fetching all submissions to update solved problems state...")
            pending_problems = set()
            try:
                submissions = self.client.get_submissions(
                    state["contest_id"],
                    state["participant_id"],
                    None  
                )
                
                solved_problems = set()
                submitted_problems = set()
                
                for sub in submissions:
                    submitted_problems.add(sub.problem_id)
                    if sub.status.value == "SUBMISSION_STATUS_ACCEPTED":
                        solved_problems.add(sub.problem_id)
                        self.logger.info(f"[{self.model_name}] Problem {sub.problem_id} is ACCEPTED")
                    elif sub.status.value == "SUBMISSION_STATUS_PENDING":
                        pending_problems.add(sub.problem_id)
                        self.logger.info(f"[{self.model_name}] Problem {sub.problem_id} has PENDING submission")
                
                state["solved_problems"] = list(solved_problems)
                state["submitted_problems"] = list(submitted_problems)
                
                self.logger.info(f"[{self.model_name}] Updated state from submissions - Solved: {len(solved_problems)}, Submitted: {len(submitted_problems)}, Pending: {len(pending_problems)}")
                
            except Exception as e:
                self.logger.warning(f"[{self.model_name}] Failed to fetch submissions: {e}")
            
            solved = state["solved_problems"]
            self.logger.info(f"[{self.model_name}] Final solved problems: {len(solved)} - {solved}")

            solved_ids = [str(pid) for pid in solved]
            pending_ids = [str(pid) for pid in pending_problems]
            
            available_problems = [p for p in all_problems if p.id not in solved_ids and p.id not in pending_ids]
            
            if pending_problems:
                self.logger.info(f"[{self.model_name}] Excluding {len(pending_problems)} problems with pending submissions: {pending_ids}")
            
            self.logger.info(f"[{self.model_name}] Problems available for selection (excluding only solved): {len(available_problems)}")
            if available_problems:
                problem_names = [p.name for p in available_problems]
                self.logger.info(f"[{self.model_name}] Available problem names: {problem_names}")
            
            if not available_problems:
                state["current_step"] = "no_problems"
                self.logger.info(f"[{self.model_name}] All problems have been solved!")
                return state
            
            info_prompt = f"""
            You are participating in a competitive programming contest. You need to select which problem to solve next.

            Contest Information:
            - Contest ID: {state["contest_id"]}
            - Time remaining: {state["remaining_time"]} seconds
            - Problems solved: {len(solved)}/{len(all_problems)}
            - Solved problem IDs: {solved}

            Available Problems: {len(available_problems)} unsolved problems

            You have access to these tools to help you decide:
            - view_leaderboard: See what other participants have solved
            - select_problem: Get detailed info about available problems with descriptions
            - view_problem: Get full details of a specific problem

            Use the available tools to gather information about the problems. Consider:
            1. What problems have other participants solved (check leaderboard)
            2. Problem difficulty and descriptions
            3. Time remaining in the contest
            4. Problem types that might be easier/faster to implement
            """
            
            messages = self._get_messages_with_context(state, HumanMessage(content=info_prompt))
            response, messages = await self._handle_tool_calls(messages, max_iterations=5)
            

            selection_prompt = f"""
            Based on the information you gathered, select which problem to solve next.
            
            Available problems are numbered 1 to {len(available_problems)}.
            Respond with the problem number (1-{len(available_problems)}) and your reasoning.
            """
            
            messages.append(HumanMessage(content=selection_prompt))
            
            try:
                # Use structured output for the selection
                structured_llm = self.llm_with_tools.with_structured_output(ProblemSelection)
                selection = await structured_llm.ainvoke(messages)
                
                problem_number = selection.problem_number
                reasoning = selection.reasoning
                
                if 1 <= problem_number <= len(available_problems):
                    selected_problem = available_problems[problem_number - 1]
                    self.logger.info(
                        f"[{self.model_name}] LLM selected problem {problem_number}/{len(available_problems)}: "
                        f"{selected_problem.name}"
                    )
                else:
                    selected_problem = available_problems[0]
                    self.logger.warning(
                        f"[{self.model_name}] LLM selected invalid problem number: {problem_number}, "
                        f"available: 1-{len(available_problems)}, using first problem"
                    )
                    reasoning = f"Invalid selection ({problem_number}), using first available problem"
                    
            except Exception as e:
                selected_problem = available_problems[0]
                reasoning = "Failed to get structured selection, using first available problem"
                self.logger.warning(f"[{self.model_name}] Structured output failed: {e}, using first problem")
            
            state["current_problem"] = selected_problem
            self.logger.info(f"[{self.model_name}] Selected problem: {selected_problem.name} (ID: {selected_problem.id})")
            self.logger.info(f"[{self.model_name}] Problem description length: {len(selected_problem.description)} chars")
            
            view_problem_tool = next(t for t in self.contest_tools if t.name == "view_problem")
            problem_details = view_problem_tool.run({
                "contest_id": state["contest_id"],
                "problem_id": selected_problem.id
            })
            
            problem_prompt = f"""
            Selected Problem: {selected_problem.name} (ID: {selected_problem.id})
            Selection Reasoning: {reasoning}
            
            {problem_details}
            """
            state["messages"].append(AIMessage(content=problem_prompt))
            self.logger.info(f"[{self.model_name}] Selection reasoning: {reasoning}")
            
        except Exception as e:
            self.logger.error(f"[{self.model_name}] Error selecting problem: {e}")
            self.state_manager.set_error(state, str(e))
        
        return state
    
    @traceable(name="solve_problem")
    async def solve_problem(self, state: AgentState) -> AgentState:

        if state.get("current_problem") is None:
            self.logger.error(f"[{self.model_name}] Cannot solve problem: current_problem is None")
            state["error_message"] = "No problem selected"
            state["current_step"] = "contest_ended"
            return state
        
        self.logger.info(f"[{self.model_name}] Solving problem {state['current_problem'].id}")
        self.logger.info(f"[{self.model_name}] Problem name: {state['current_problem'].name}")
        self.logger.info(f"[{self.model_name}] Problem description length: {len(state['current_problem'].description)} chars")
        
        state["current_step"] = "solve_problem"
        self.state_manager.update_state(state)
        
        try:
            analysis_prompt = f"""
            Analyze this competitive programming problem and provide a structured analysis:
            
            Problem: {state['current_problem'].name}
            Description: {state['current_problem'].description}
            Time Limit: {state['current_problem'].time_limit_ms}ms
            Memory Limit: {state['current_problem'].memory_limit_mb}MB
            
            Provide:
            1. Your understanding of the problem
            2. Proposed solution approach
            3. Confidence level (1-10)
            """
            
            messages = self._get_messages_with_context(state, HumanMessage(content=analysis_prompt))
            analysis_response = await self._call_llm_with_retry(messages, structured_output=ProblemAnalysis)
            
            if analysis_response is None:
                raise ValueError("LLM returned None for problem analysis")
            
            confidence = analysis_response.confidence if analysis_response.confidence else 5
            self.logger.info(f"[{self.model_name}] Problem analysis - Confidence: {confidence}/10")
            self.logger.info(f"[{self.model_name}] Approach: {analysis_response.approach}")
            state["current_step"] = "coding"
            self.state_manager.update_state(state)
            
            self.logger.info(f"[{self.model_name}] Coding solution for problem: {state['current_problem'].name}")
            
            
            solution_prompt = f"""
            Based on the analysis, implement a solution for this competitive programming problem:
            
            Problem: {state['current_problem'].name}
            Description: {state['current_problem'].description}
            Time Limit: {state['current_problem'].time_limit_ms}ms
            Memory Limit: {state['current_problem'].memory_limit_mb}MB
            
            
            Requirements:
            - Choose the most appropriate language: C++ or Python
            - C++ is preferred for performance-critical problems or when speed is essential
            - Python is preferred for problems involving complex data structures, string manipulation, or when implementation speed matters more than execution speed
            - Include all necessary headers/imports
            - Handle input/output correctly (read from stdin, write to stdout)
            - Consider edge cases
            - Optimize for time and space complexity
            
            Provide the complete, runnable code in your chosen language.
            """
            
            messages = self._get_messages_with_context(state, HumanMessage(content=solution_prompt))
            solution_response = await self._call_llm_with_retry(messages, structured_output=CodeSolution)
            
 
            if solution_response is None:
                raise ValueError("LLM returned None for solution")
            if not hasattr(solution_response, 'language') or not hasattr(solution_response, 'code'):
                raise ValueError("LLM solution response missing required fields")
            if not solution_response.code or len(solution_response.code.strip()) < 10:
                raise ValueError(f"LLM generated invalid/empty code: {len(solution_response.code)} chars")
            
            self.logger.info(f"[{self.model_name}] Generated {solution_response.language} solution")
            
            log_contest_event(
                "problem_solved",
                state["contest_id"],
                state["participant_id"],
                data={
                    "problem_id": state["current_problem"].id,
                    "problem_name": state["current_problem"].name,
                    "language": solution_response.language,
                    "code_length": len(solution_response.code),
                    "confidence": confidence
                }
            )
            
            state["solution"] = solution_response
            state["extracted_code"] = solution_response.code
            state["solution_language"] = solution_response.language
            
        except Exception as e:
            self.logger.error(f"[{self.model_name}] Error solving problem: {e}")
            self.state_manager.set_error(state, str(e))
        
        return state
    
    @traceable(name="submit_solution")
    async def submit_solution(self, state: AgentState) -> AgentState:
        self.logger.info(f"[{self.model_name}] Submitting solution")
        
        state["current_step"] = "submit_solution"
        self.state_manager.update_state(state)
        
        try:
            if not state.get("extracted_code"):
                raise Exception("No solution code available to submit")
            
            code = state["extracted_code"]
            language = state.get("solution_language", "cpp")
            
            self.logger.info(f"[{self.model_name}] Using structured solution - Language: {language}")
            self.logger.info(f"[{self.model_name}] Code length: {len(code)} characters")
            
            submit_tool = next(t for t in self.contest_tools if t.name == "submit_solution")
            result = submit_tool.run({
                "contest_id": state["contest_id"],
                "participant_id": state["participant_id"],
                "problem_id": state["current_problem"].id,
                "code": code,
                "language": language
            })
            
            if "successfully" in result.lower() or "submitted" in result.lower():
                problem_id = state["current_problem"].id
                self.state_manager.add_submitted_problem(state, problem_id)
                
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
            
        except Exception as e:
            error_msg = str(e)
            self.logger.error(f"[{self.model_name}] Error submitting solution: {error_msg}")
            
            if not self._handle_contest_end_error(state, error_msg, "submission"):
                self.state_manager.set_error(state, error_msg)
        
        return state
    
    @traceable(name="check_results")
    async def check_results(self, state: AgentState) -> AgentState:
        self.logger.info(f"[{self.model_name}] Checking submission results")
        
        state["current_step"] = "check_results"
        self.state_manager.update_state(state)
        
        try:
            check_tool = next(t for t in self.contest_tools if t.name == "check_submission_results")
            tool_args = {
                "contest_id": state["contest_id"],
                "participant_id": state["participant_id"],
                "problem_id": state["current_problem"].id
            }
            self.logger.info(f"[{self.model_name}] Calling tool with args: {tool_args}")
            
            result_msg = check_tool.run(tool_args)
            self.logger.info(f"[{self.model_name}] Tool returned: {result_msg}")
            
            if "error" in result_msg:
                raise ValueError(f"Error checking submission results: {result_msg}")

            if "🎉 PROBLEM SOLVED! ✅" in result_msg:
                problem_id = state["current_problem"].id
                self.logger.info(f"[{self.model_name}] PROBLEM SOLVED detected! Adding {problem_id} to solved_problems")
                if self.state_manager.add_solved_problem(state, problem_id):
                    self.logger.info(f"[{self.model_name}] Successfully added problem {problem_id} to solved list. Total solved: {len(state['solved_problems'])}")
                    self.logger.info(f"[{self.model_name}] Solved problems list: {state['solved_problems']}")
                else:
                    self.logger.warning(f"[{self.model_name}] Problem {problem_id} was already in solved list")
                self.state_manager.clear_error(state)
            else:
                self.logger.info(f"[{self.model_name}] Problem not solved yet. Result message: {result_msg[:100]}...")
            
            state["messages"].append(AIMessage(content=result_msg))
            self.logger.info(f"[{self.model_name}] {result_msg}")
            
        except Exception as e:
            error_msg = str(e)
            self.logger.error(f"[{self.model_name}] Error checking results: {error_msg}")
            
            if not self._handle_contest_end_error(state, error_msg, "check_results"):
                self.state_manager.set_error(state, error_msg)
        
        return state

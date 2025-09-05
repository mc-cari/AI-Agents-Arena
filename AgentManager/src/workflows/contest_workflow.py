from typing import TypedDict, List, Optional, Annotated
from langgraph.graph import StateGraph, END
from langchain_core.messages import BaseMessage, HumanMessage, AIMessage
from langchain_core.language_models import BaseChatModel
from datetime import datetime, timedelta
import logging
import asyncio

from ..models import Contest, Problem, AgentConfig
from ..grpc_client import ContestManagerClient
from ..tools import create_contest_tools, create_coding_tools


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


class ContestAgent:

    def __init__(self, config: AgentConfig, client: ContestManagerClient, llm: BaseChatModel):
        self.config = config
        self.client = client
        self.llm = llm
        self.logger = logging.getLogger(__name__)
        
        self.contest_tools = create_contest_tools(client)
        self.coding_tools = create_coding_tools()
        self.all_tools = self.contest_tools + self.coding_tools
        
        self.llm_with_tools = llm.bind_tools(self.all_tools)
        
        self.workflow = self._create_workflow()
    
    def _create_workflow(self) -> StateGraph:
        workflow = StateGraph(AgentState)
        
        workflow.add_node("analyze_contest", self._analyze_contest)
        workflow.add_node("select_problem", self._select_problem)
        workflow.add_node("solve_problem", self._solve_problem)
        workflow.add_node("submit_solution", self._submit_solution)
        workflow.add_node("check_results", self._check_results)
        workflow.add_node("monitor_contest", self._monitor_contest)
        
        workflow.add_edge("analyze_contest", "select_problem")
        workflow.add_edge("select_problem", "solve_problem")
        workflow.add_edge("solve_problem", "submit_solution")
        workflow.add_edge("submit_solution", "check_results")
        
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
            
            response = await self.llm.ainvoke(state["messages"])
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
        self.logger.info(f"Solving problem {state['current_problem'].id}")
        
        try:
            problem = state["current_problem"]
            
            solve_prompt = f"""
            Now solve this problem step by step using the available tools:
            
            Problem: {problem.name}
            Description: {problem.description}
            
            Available tools to help you:
            1. analyze_problem - Analyze the problem and suggest approaches
            2. view_problem - View detailed problem information
            
            IMPORTANT: You must provide a COMPLETE, WORKING solution to this problem.
            
            Steps to follow:
            1. First, use analyze_problem to understand the problem requirements
            2. View the problem details to understand input/output format and constraints
            3. Develop a complete solution in C++20 (preferably) or Python 3.13
            4. Your solution must:
               - Handle the input format correctly
               - Implement the complete algorithm
               - Handle edge cases
               - Output the result in the expected format
               - Be ready to run without additional modifications
            
            Write the complete solution directly in your response.
            Your solution should be a complete, runnable program that solves the given problem.
            
            Format your solution as:
            
            ```cpp
            // Complete solution here
            ```
            or
            ```python
            # Complete solution here
            ```
            """
            
            state["messages"].append(HumanMessage(content=solve_prompt))
            response = await self.llm_with_tools.ainvoke(state["messages"])
            state["messages"].append(response)
            
            if hasattr(response, 'tool_calls') and response.tool_calls:
                for tool_call in response.tool_calls:
                    tool_result = await self._execute_tool(tool_call)
                    
                    from langchain_core.messages import ToolMessage
                    tool_message = ToolMessage(
                        content=tool_result,
                        tool_call_id=tool_call["id"]
                    )
                    state["messages"].append(tool_message)
                    

                    solution_prompt = f"""
                    Based on the problem analysis and your understanding, now provide the COMPLETE solution.
                    
                    You must write a complete, working program that:
                    1. Reads the input correctly
                    2. Implements the algorithm to solve the problem
                    3. Outputs the result in the expected format
                    4. Handles all edge cases mentioned in the problem
                    
                    Write your complete solution as a runnable program, not as a template or partial code.
                    """
                    
                    state["messages"].append(HumanMessage(content=solution_prompt))
                    solution_response = await self.llm.ainvoke(state["messages"])
                    state["messages"].append(solution_response)
                    
                    if solution_response.content:
                        extracted_code = self._extract_code_from_message(solution_response.content)
                        if not extracted_code:
                            retry_prompt = """
                            I need you to provide the actual code solution, not just an explanation.
                            Please write the complete, runnable program that solves the problem.
                            Format it as a code block with ```python or ```cpp.
                            """
                            state["messages"].append(HumanMessage(content=retry_prompt))
                            retry_response = await self.llm.ainvoke(state["messages"])
                            state["messages"].append(retry_response)
            
            state["current_step"] = "solve_problem"
            
        except Exception as e:
            self.logger.error(f"Error solving problem: {e}")
            state["error_message"] = str(e)
        
        return state
    
    async def _submit_solution(self, state: AgentState) -> AgentState:
        self.logger.info("Submitting solution")
        
        try:
            code = None
            for i in range(len(state["messages"]) - 1, max(-1, len(state["messages"]) - 10), -1):
                if i < 0:
                    break
                message = state["messages"][i]
                if hasattr(message, 'content') and message.content:
                    self.logger.info(f"Checking message {i} for code...")
                    extracted_code = self._extract_code_from_message(message.content)
                    if extracted_code:
                        code = extracted_code
                        self.logger.info(f"Found code in message {i}")
                        self.logger.info(f"Code preview: {extracted_code[:200]}...")
                        break
            
            if code:
                if any(keyword in code.lower() for keyword in ['def ', 'import ', 'if __name__', 'print(']):
                    language = "python"
                elif any(keyword in code.lower() for keyword in ['#include', 'int main', 'using namespace', 'cout', 'cin']):
                    language = "cpp"
                else:
                    language = "python" if "def " in code or "import " in code else "cpp"
                
                self.logger.info(f"Detected language: {language}")
                self.logger.info(f"Code length: {len(code)} characters")
                self.logger.info(f"Code starts with: {code[:100]}...")
                
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
                else:
                    raise Exception(f"Submit tool failed: {result}")
            else:
                self.logger.warning("No code found in recent messages. Last few messages:")
                for i in range(max(0, len(state["messages"]) - 5), len(state["messages"])):
                    msg = state["messages"][i]
                    if hasattr(msg, 'content'):
                        self.logger.warning(f"Message {i}: {msg.content[:300]}...")
                    else:
                        self.logger.warning(f"Message {i}: {type(msg)}")
                
                self.logger.warning("Attempting to find any code-like content...")
                for i in range(len(state["messages"]) - 1, max(-1, len(state["messages"]) - 15), -1):
                    if i < 0:
                        break
                    msg = state["messages"][i]
                    if hasattr(msg, 'content') and msg.content:
                        if any(keyword in msg.content.lower() for keyword in ['def ', 'import ', '#include', 'int main', 'class ']):
                            self.logger.warning(f"Found potential code in message {i}: {msg.content[:200]}...")
                
                raise Exception("No code found in the solution")
            
            state["current_step"] = "submit_solution"
            
        except Exception as e:
            self.logger.error(f"Error submitting solution: {e}")
            state["error_message"] = str(e)
        
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
            self.logger.error(f"Error checking results: {e}")
            state["error_message"] = str(e)
        
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
        if state["remaining_time"] <= 20:
            return "end"
        
        contest = state["contest_info"]
        if len(state["solved_problems"]) >= len(contest.problems):
            self.logger.info(f"All {len(contest.problems)} problems solved! Contest complete!")
            return "end"
        
        if state["error_message"]:
            return "select_new"

        return "continue"
    
    def _check_time_remaining(self, state: AgentState) -> str:
        contest = state["contest_info"]
        now = datetime.now()
        remaining = (contest.ends_at - now).total_seconds()
        state["remaining_time"] = max(0, int(remaining))

        if state["remaining_time"] <= 20:
            return "end"
        
        return "continue"
    
    def _extract_code_from_message(self, content: str) -> Optional[str]:
        self.logger.debug(f"Extracting code from message of length {len(content)}")
        import re
        

        code_pattern = r'```(?:python|cpp|c\+\+)?\n(.*?)\n```'
        matches = re.findall(code_pattern, content, re.DOTALL)
        
        if matches:
            code = matches[-1].strip()
            if len(code) > 20:
                return code
        
        generic_code_pattern = r'```\n(.*?)\n```'
        generic_matches = re.findall(generic_code_pattern, content, re.DOTALL)
        
        if generic_matches:
            code = generic_matches[-1].strip()
            if len(code) > 50:
                return code
        
        python_patterns = [
            r'(?:import\s+.*?\n)*\s*(?:def\s+\w+\s*\([^)]*\)\s*:.*?)(?=\n\S|$)',
            r'(?:import\s+.*?\n)*\s*(?:class\s+\w+.*?)(?=\n\S|$)',
            r'(?:import\s+.*?\n)*\s*(?:if\s+__name__\s*==\s*[\'"]__main__[\'"].*?)(?=\n\S|$)'
        ]
        
        for pattern in python_patterns:
            matches = re.findall(pattern, content, re.DOTALL)
            if matches:
                code = matches[-1].strip()
                if len(code) > 50:
                    return code
        
        cpp_patterns = [
            r'#include\s+<.*?>\n(?:#include\s+<.*?>\n)*\s*(?:using\s+namespace\s+std;)?\s*(?:int\s+main\s*\([^)]*\)\s*\{.*?\})',
            r'(?:int|void|string|bool)\s+\w+\s*\([^)]*\)\s*\{.*?\}',
            r'#include\s+<.*?>\n.*?int\s+main\s*\([^)]*\)\s*\{.*?\}'
        ]
        
        for pattern in cpp_patterns:
            matches = re.findall(pattern, content, re.DOTALL)
            if matches:
                code = matches[-1].strip()
                if len(code) > 50:
                    return code
        
        code_keywords = ['def ', 'class ', 'import ', '#include', 'int ', 'void ', 'string ', 'bool ', 'main(', 'if __name__', 'print(', 'cout', 'cin', 'return ']
        lines = content.split('\n')
        code_lines = []
        in_code = False
        code_started = False
        
        for line in lines:
            line_stripped = line.strip()
            
            # Check if we're starting a code block
            if any(keyword in line_stripped for keyword in code_keywords):
                if not code_started:
                    code_started = True
                    in_code = True
                else:
                    in_code = True
            
            # If we're in code and hit a blank line, check if we should continue
            if in_code and not line_stripped:
                # Look ahead to see if more code follows
                next_non_empty = None
                for next_line in lines[lines.index(line) + 1:]:
                    if next_line.strip():
                        next_non_empty = next_line.strip()
                        break
                
                # If next non-empty line doesn't look like code, stop
                if next_non_empty and not any(keyword in next_non_empty for keyword in code_keywords):
                    break
            
            if in_code:
                code_lines.append(line)
        
        if code_lines and len('\n'.join(code_lines)) > 50:
            return '\n'.join(code_lines)
        
        potential_code_sections = []
        current_section = []
        
        for line in lines:
            line_stripped = line.strip()
            
            # If line looks like code, add to current section
            if (any(keyword in line_stripped for keyword in code_keywords) or
                line_stripped.startswith('#') or
                line_stripped.startswith('//') or
                line_stripped.startswith('/*') or
                line_stripped.startswith('*') or
                line_stripped.endswith(':') or
                line_stripped.endswith('{') or
                line_stripped.endswith('}') or
                line_stripped.endswith(';') or
                line_stripped.endswith(')') or
                line_stripped.endswith(']')):
                
                current_section.append(line)
            elif current_section:
                # If we have a current section and hit a non-code line, save it
                if len(current_section) > 2:  # At least 3 lines to be substantial
                    potential_code_sections.append('\n'.join(current_section))
                current_section = []
        
        # Add the last section if it exists
        if current_section and len(current_section) > 2:
            potential_code_sections.append('\n'.join(current_section))
        
        # Return the longest potential code section
        if potential_code_sections:
            longest_section = max(potential_code_sections, key=len)
            self.logger.debug(f"Found potential code section of length {len(longest_section)}")
            return longest_section
        
        self.logger.debug("No code found in message")
        return None
    
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
            config = {"recursion_limit": 100} 
            final_state = await self.workflow.ainvoke(initial_state, config=config)
            self.logger.info(f"Contest participation completed. Final step: {final_state['current_step']}")
        except Exception as e:
            self.logger.error(f"Error during contest participation: {e}")
            raise

from langchain_core.tools import BaseTool
from typing import Optional, Type, List
from pydantic import BaseModel, Field

from ..grpc_client import ContestManagerClient
from ..models import Language


class ContestTool(BaseTool):
    
    def __init__(self, client: ContestManagerClient, **kwargs):
        super().__init__(**kwargs)
        self._client = client
    
    @property
    def client(self) -> ContestManagerClient:
        return self._client


class ViewLeaderboardInput(BaseModel):
    contest_id: str = Field(description="The ID of the contest")


class ViewLeaderboardTool(ContestTool):
    name: str = "view_leaderboard"
    description: str = "Get the current leaderboard showing participant rankings and scores"
    args_schema: Type[BaseModel] = ViewLeaderboardInput
    
    def _run(self, contest_id: str) -> str:
        participants = self.client.get_leaderboard(contest_id)
        
        if not participants:
            return "No leaderboard data available."
        
        participants.sort(key=lambda p: p.result.rank)
        
        leaderboard = "Current Leaderboard:\n"
        leaderboard += "Rank | Participant | Solved | Penalty\n"
        leaderboard += "-----|-----------|---------|---------\n"
        
        for p in participants:
            leaderboard += f"{p.result.rank:4d} | {p.model_name:11s} | {p.result.solved:6d} | {p.result.total_penalty_seconds:7d}\n"
        
        return leaderboard


class ViewProblemInput(BaseModel):
    contest_id: str = Field(description="The ID of the contest")
    problem_id: str = Field(description="The ID of the problem to view")


class ViewProblemTool(ContestTool):
    name: str = "view_problem"
    description: str = "Get the description and constraints of a specific problem"
    args_schema: Type[BaseModel] = ViewProblemInput
    
    def _run(self, contest_id: str, problem_id: str) -> str:
        contest = self.client.get_contest(contest_id)
        
        if not contest:
            return "Contest not found."
        
        problem = next((p for p in contest.problems if p.id == problem_id), None)
        
        if not problem:
            return f"Problem {problem_id} not found in contest."
        
        problem_info = f"Problem: {problem.name}\n"
        problem_info += f"ID: {problem.id}\n"
        problem_info += f"Time Limit: {problem.time_limit_ms}ms\n"
        problem_info += f"Memory Limit: {problem.memory_limit_mb}MB\n\n"
        problem_info += f"Description:\n{problem.description}"
        
        return problem_info


class ViewSubmissionsInput(BaseModel):
    contest_id: str = Field(description="The ID of the contest")
    participant_id: str = Field(description="The ID of the participant")
    problem_id: Optional[str] = Field(default=None, description="Optional problem ID to filter submissions")


class ViewSubmissionsTool(ContestTool):
    name: str = "view_submissions"
    description: str = "Get the list of submissions made by a participant for a contest or specific problem"
    args_schema: Type[BaseModel] = ViewSubmissionsInput
    
    def _run(self, contest_id: str, participant_id: str, problem_id: Optional[str] = None) -> str:
        submissions = self.client.get_submissions(contest_id, participant_id, problem_id)
        
        if not submissions:
            return "No submissions found."
        
        result = f"Submissions for participant {participant_id}:\n"
        result += "Time     | Problem | Language | Status\n"
        result += "---------|---------|----------|--------\n"
        
        for sub in submissions:
            time_str = sub.submitted_at.strftime("%H:%M:%S")
            result += f"{time_str} | {sub.problem_id:7s} | {sub.language.value:8s} | {sub.status.value}\n"
            if sub.verdict_message:
                result += f"         | Verdict: {sub.verdict_message}\n"
        
        return result


class SubmitSolutionInput(BaseModel):
    contest_id: str = Field(description="The ID of the contest")
    participant_id: str = Field(description="The ID of the participant")
    problem_id: str = Field(description="The ID of the problem")
    code: str = Field(description="The solution code")
    language: str = Field(description="The programming language (python or cpp)")


class SubmitSolutionTool(ContestTool):
    name: str = "submit_solution"
    description: str = "Submit a solution code for a specific problem"
    args_schema: Type[BaseModel] = SubmitSolutionInput
    
    def _run(self, contest_id: str, participant_id: str, problem_id: str, 
             code: str, language: str) -> str:
        if language.lower() == "python":
            lang_enum = Language.PYTHON
        elif language.lower() == "cpp":
            lang_enum = Language.CPP
        else:
            return f"Unsupported language: {language}. Use 'python' or 'cpp'."
        
        submission = self.client.submit_solution(
            contest_id, participant_id, problem_id, code, lang_enum
        )
        
        if not submission:
            return "Failed to submit solution."
        
        return f"Solution submitted successfully! Submission ID: {submission.id}"


class ViewContestInput(BaseModel):
    contest_id: str = Field(description="The ID of the contest")


class ViewContestTool(ContestTool):
    
    name: str = "view_contest"
    description: str = "Get general information about the contest including problems and participants"
    args_schema: Type[BaseModel] = ViewContestInput
    
    def _run(self, contest_id: str) -> str:
        contest = self.client.get_contest(contest_id)
        
        if not contest:
            return "Contest not found."
        
        info = f"Contest ID: {contest.id}\n"
        info += f"State: {contest.state.value}\n"
        info += f"Started: {contest.started_at.strftime('%Y-%m-%d %H:%M:%S')}\n"
        info += f"Ends: {contest.ends_at.strftime('%Y-%m-%d %H:%M:%S')}\n"
        info += f"Participants: {len(contest.participants)}\n"
        info += f"Problems: {len(contest.problems)}\n\n"
        
        info += "Problems:\n"
        for i, problem in enumerate(contest.problems, 1):
            info += f"{i}. {problem.name} (ID: {problem.id})\n"
        
        info += "\nParticipants:\n"
        for i, participant in enumerate(contest.participants, 1):
            info += f"{i}. {participant.model_name} (ID: {participant.id})\n"

        return info

class SelectProblemInput(BaseModel):
    contest_id: str = Field(description="The ID of the contest")
    solved_problems: List[str] = Field(description="List of already solved problem IDs")
    time_remaining: int = Field(description="Time remaining in seconds")


class SelectProblemTool(ContestTool):
    name: str = "select_problem"
    description: str = "Get available problems formatted for LLM selection with competitive analysis"
    args_schema: Type[BaseModel] = SelectProblemInput
    
    def _run(self, contest_id: str, solved_problems: List[str], time_remaining: int) -> str:
        contest = self.client.get_contest(contest_id)
        if not contest:
            return "Contest not found."
        
        available_problems = [p for p in contest.problems if p.id not in solved_problems]
        
        if not available_problems:
            return "No problems available. All problems have been solved!"
        
        selection_info = f"""Contest Problem Selection:
Time remaining: {time_remaining} seconds
Problems already solved: {len(solved_problems)}
Problems available: {len(available_problems)}

Available Problems:
"""
        
        for i, problem in enumerate(available_problems):
            selection_info += f"""
Problem {i+1}:
- ID: {problem.id}
- Name: {problem.name}
- Time Limit: {problem.time_limit_ms}ms
- Memory Limit: {problem.memory_limit_mb}MB
- Description: {problem.description[:200]}{'...' if len(problem.description) > 200 else ''}
"""
        
        selection_info += f"""
Respond with ONLY the problem number (1-{len(available_problems)}) that you want to solve, followed by a brief explanation on the next line.

Example response:
2
This problem appears to be a straightforward implementation problem that can be solved quickly.
"""
        
        return selection_info


class CheckSubmissionResultsInput(BaseModel):
    contest_id: str = Field(description="The ID of the contest")
    participant_id: str = Field(description="The ID of the participant")
    problem_id: str = Field(description="The ID of the problem")


class CheckSubmissionResultsTool(ContestTool):
    name: str = "check_submission_results"
    description: str = "Check the status and verdict of the latest submission for a problem"
    args_schema: Type[BaseModel] = CheckSubmissionResultsInput
    
    def _run(self, contest_id: str, participant_id: str, problem_id: str) -> str:
        submissions = self.client.get_submissions(contest_id, participant_id, problem_id)
        
        if not submissions:
            return "No submissions found for this problem."
        
        latest = submissions[-1]
        result_msg = f"Latest submission for problem {problem_id}:\n"
        result_msg += f"Status: {latest.status.value}\n"
        result_msg += f"Submitted at: {latest.submitted_at.strftime('%H:%M:%S')}\n"
        
        if latest.verdict_message:
            result_msg += f"Verdict: {latest.verdict_message}\n"
        
        if latest.status.value.upper() == "ACCEPTED" or "ACCEPTED" in latest.status.value.upper():
            result_msg += "🎉 PROBLEM SOLVED! ✅"
        else:
            result_msg += "❌ Problem not yet solved"
        
        return result_msg


def create_contest_tools(client: ContestManagerClient) -> List[BaseTool]:
    return [
        ViewContestTool(client),
        ViewLeaderboardTool(client),
        ViewProblemTool(client),
        ViewSubmissionsTool(client),
        SubmitSolutionTool(client),
        SelectProblemTool(client),
        CheckSubmissionResultsTool(client),
    ]

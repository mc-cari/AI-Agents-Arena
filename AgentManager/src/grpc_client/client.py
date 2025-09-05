import grpc
import logging
import sys
import os
from typing import List, Optional
from datetime import datetime

# Add current directory to path for protobuf imports
sys.path.append(os.path.dirname(__file__))

import contest_pb2
import contest_pb2_grpc

from ..models import Contest, Problem, Participant, Submission, Language, ContestState, SubmissionStatus, ProblemResult, ParticipantResult, ProblemStatus


class ContestManagerClient:
    """gRPC client for communicating with ContestManager service."""
    
    def __init__(self, host: str = "localhost:50051"):
        self.host = host
        self.channel = None
        self.stub = None
        self.logger = logging.getLogger(__name__)
    
    def connect(self):
        """Establish connection to ContestManager service."""
        try:
            self.channel = grpc.insecure_channel(self.host)
            self.stub = contest_pb2_grpc.ContestServiceStub(self.channel)
            self.logger.info(f"Connected to ContestManager at {self.host}")
        except Exception as e:
            self.logger.error(f"Failed to connect to ContestManager: {e}")
            raise
    
    def disconnect(self):
        """Close connection to ContestManager service."""
        if self.channel:
            self.channel.close()
            self.logger.info("Disconnected from ContestManager")
    
    def create_contest(self, num_problems: int, participant_models: List[str]) -> Optional[Contest]:
        """Create a new contest with specified problems and participants."""
        try:
            request = contest_pb2.CreateContestRequest(
                num_problems=num_problems,
                participant_models=participant_models
            )
            response = self.stub.CreateContest(request)
            self.logger.info(f"Created contest: {response.contest.id}")
            return self._convert_contest(response.contest)
        except grpc.RpcError as e:
            self.logger.error(f"Failed to create contest: {e}")
            return None

    def create_contest_with_problems(self, problem_ids: List[str], participant_models: List[str]) -> Optional[Contest]:
        try:
            request = contest_pb2.CreateContestWithProblemsRequest(
                problem_ids=problem_ids,
                participant_models=participant_models
            )
            response = self.stub.CreateContestWithProblems(request)
            self.logger.info(f"Created contest with specific problems: {response.contest.id}")
            return self._convert_contest(response.contest)
        except grpc.RpcError as e:
            self.logger.error(f"Failed to create contest with problems: {e}")
            return None

    def get_contest(self, contest_id: str) -> Optional[Contest]:
        """Get contest information by ID."""
        try:
            request = contest_pb2.GetContestRequest(contest_id=contest_id)
            response = self.stub.GetContest(request)
            return self._convert_contest(response.contest)
        except grpc.RpcError as e:
            self.logger.error(f"Failed to get contest {contest_id}: {e}")
            return None

    def list_contests(self, page_size: int = 50, page_token: str = "") -> tuple[List[Contest], str]:
        """List all contests with pagination."""
        try:
            request = contest_pb2.ListContestsRequest(
                page_size=page_size,
                page_token=page_token
            )
            response = self.stub.ListContests(request)
            contests = [self._convert_contest(c) for c in response.contests]
            return contests, response.next_page_token
        except grpc.RpcError as e:
            self.logger.error(f"Failed to list contests: {e}")
            return [], ""
    
    def submit_solution(self, contest_id: str, participant_id: str, problem_id: str, 
                       code: str, language: Language) -> Optional[Submission]:
        """Submit a solution to a problem."""
        try:
            lang_enum = contest_pb2.LANGUAGE_PYTHON if language == Language.PYTHON else contest_pb2.LANGUAGE_CPP
            request = contest_pb2.SubmitSolutionRequest(
                contest_id=contest_id,
                participant_id=participant_id,
                problem_id=problem_id,
                code=code,
                language=lang_enum
            )
            response = self.stub.SubmitSolution(request)
            return self._convert_submission(response.submission)
        except grpc.RpcError as e:
            self.logger.error(f"Failed to submit solution: {e}")
            return None
    
    def get_submissions(self, contest_id: str, participant_id: str, 
                       problem_id: Optional[str] = None) -> List[Submission]:
        """Get submissions for a participant."""
        try:
            request = contest_pb2.GetSubmissionsRequest(
                contest_id=contest_id,
                participant_id=participant_id,
                problem_id=problem_id or ""
            )
            response = self.stub.GetSubmissions(request)
            return [self._convert_submission(sub) for sub in response.submissions]
        except grpc.RpcError as e:
            self.logger.error(f"Failed to get submissions: {e}")
            return []
    
    def get_leaderboard(self, contest_id: str) -> List[Participant]:
        """Get current leaderboard for a contest."""
        try:
            request = contest_pb2.GetLeaderboardRequest(contest_id=contest_id)
            response = self.stub.GetLeaderboard(request)
            return [self._convert_participant(p) for p in response.participants]
        except grpc.RpcError as e:
            self.logger.error(f"Failed to get leaderboard: {e}")
            return []
    
    def _convert_contest(self, pb_contest) -> Contest:
        """Convert protobuf Contest to our Contest model."""
        # Handle ContestState enum correctly
        if pb_contest.state == contest_pb2.CONTEST_STATE_RUNNING:
            state = ContestState.RUNNING
        elif pb_contest.state == contest_pb2.CONTEST_STATE_FINISHED:
            state = ContestState.FINISHED
        else:
            state = ContestState.RUNNING  # Default
            
        # Handle optional timestamp fields safely
        started_at = None
        ends_at = None
        
        if hasattr(pb_contest, 'started_at') and pb_contest.started_at:
            started_at = datetime.fromtimestamp(pb_contest.started_at.seconds)
        if hasattr(pb_contest, 'ends_at') and pb_contest.ends_at:
            ends_at = datetime.fromtimestamp(pb_contest.ends_at.seconds)
        
        return Contest(
            id=pb_contest.id,
            state=state,
            started_at=started_at,
            ends_at=ends_at,
            problems=[self._convert_problem(p) for p in pb_contest.problems] if pb_contest.problems else [],
            participants=[self._convert_participant(p) for p in pb_contest.participants] if pb_contest.participants else []
        )
    
    def _convert_problem(self, pb_problem) -> Problem:
        """Convert protobuf Problem to our Problem model."""
        # Handle ProblemTag enum correctly
        tag_name = "IMPLEMENTATION"  # Default
        try:
            if hasattr(pb_problem, 'tag'):
                # Get the enum name from the protobuf
                tag_name = contest_pb2.ProblemTag.Name(pb_problem.tag)
                # Remove the PROBLEM_TAG_ prefix if present
                if tag_name.startswith('PROBLEM_TAG_'):
                    tag_name = tag_name[12:]
        except Exception:
            tag_name = "IMPLEMENTATION"
            
        return Problem(
            id=pb_problem.id,
            name=pb_problem.name,
            description=pb_problem.description,
            time_limit_ms=pb_problem.time_limit_ms,
            memory_limit_mb=pb_problem.memory_limit_mb,
            tag=tag_name
        )
    
    def _convert_participant(self, pb_participant) -> Participant:
        """Convert protobuf Participant to our Participant model."""
        # Handle ParticipantResult safely
        result = None
        if hasattr(pb_participant, 'result') and pb_participant.result:
            problem_results = {}
            if hasattr(pb_participant.result, 'problem_results'):
                for k, v in pb_participant.result.problem_results.items():
                    # Handle ProblemStatus enum correctly
                    status = ProblemStatus.NON_TRIED  # Default
                    try:
                        if hasattr(v, 'status'):
                            status_name = contest_pb2.ProblemStatus.Name(v.status)
                            if status_name.startswith('PROBLEM_STATUS_'):
                                status_name = status_name[15:]
                            status = ProblemStatus[status_name]
                    except Exception:
                        status = ProblemStatus.NON_TRIED
                    
                    problem_results[k] = ProblemResult(
                        status=status,
                        penalty_count=getattr(v, 'penalty_count', 0),
                        penalty_seconds=getattr(v, 'penalty_seconds', 0)
                    )
            
            result = ParticipantResult(
                solved=getattr(pb_participant.result, 'solved', 0),
                total_penalty_seconds=getattr(pb_participant.result, 'total_penalty_seconds', 0),
                problem_results=problem_results,
                rank=getattr(pb_participant.result, 'rank', 0)
            )
        
        return Participant(
            id=pb_participant.id,
            model_name=pb_participant.model_name,
            result=result
        )
    
    def _convert_submission(self, pb_submission) -> Submission:
        """Convert protobuf Submission to our Submission model."""
        # Handle Language enum correctly
        language = Language.PYTHON  # Default
        if hasattr(pb_submission, 'language'):
            if pb_submission.language == contest_pb2.LANGUAGE_CPP:
                language = Language.CPP
            elif pb_submission.language == contest_pb2.LANGUAGE_PYTHON:
                language = Language.PYTHON
        
        # Handle SubmissionStatus enum correctly
        status = SubmissionStatus.PENDING  # Default
        try:
            if hasattr(pb_submission, 'status'):
                status_name = contest_pb2.SubmissionStatus.Name(pb_submission.status)
                if status_name.startswith('SUBMISSION_STATUS_'):
                    status_name = status_name[18:]
                status = SubmissionStatus[status_name]
        except Exception:
            status = SubmissionStatus.PENDING
        
        # Handle optional timestamp
        submitted_at = None
        if hasattr(pb_submission, 'submitted_at') and pb_submission.submitted_at:
            submitted_at = datetime.fromtimestamp(pb_submission.submitted_at.seconds)
        
        return Submission(
            id=pb_submission.id,
            contest_id=pb_submission.contest_id,
            participant_id=pb_submission.participant_id,
            problem_id=pb_submission.problem_id,
            code=pb_submission.code,
            language=language,
            status=status,
            submitted_at=submitted_at,
            verdict_message=getattr(pb_submission, 'verdict_message', ''),
            total_test_cases=getattr(pb_submission, 'total_test_cases', 0),
            processed_test_cases=getattr(pb_submission, 'processed_test_cases', 0)
        )

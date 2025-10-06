from dataclasses import dataclass
from typing import List, Dict, Optional
from enum import Enum
from datetime import datetime


class Language(Enum):
    PYTHON = "LANGUAGE_PYTHON"
    CPP = "LANGUAGE_CPP"


class ContestState(Enum):
    RUNNING = "CONTEST_STATE_RUNNING"
    FINISHED = "CONTEST_STATE_FINISHED"


class SubmissionStatus(Enum):
    PENDING = "SUBMISSION_STATUS_PENDING"
    COMPILING = "SUBMISSION_STATUS_COMPILING"
    RUNNING = "SUBMISSION_STATUS_RUNNING"
    ACCEPTED = "SUBMISSION_STATUS_ACCEPTED"
    WRONG_ANSWER = "SUBMISSION_STATUS_WRONG_ANSWER"
    PRESENTATION_ERROR = "SUBMISSION_STATUS_PRESENTATION_ERROR"
    TIME_LIMIT_EXCEEDED = "SUBMISSION_STATUS_TIME_LIMIT_EXCEEDED"
    MEMORY_LIMIT_EXCEEDED = "SUBMISSION_STATUS_MEMORY_LIMIT_EXCEEDED"
    RUNTIME_ERROR = "SUBMISSION_STATUS_RUNTIME_ERROR"
    COMPILATION_ERROR = "SUBMISSION_STATUS_COMPILATION_ERROR"
    OUTPUT_LIMIT_EXCEEDED = "SUBMISSION_STATUS_OUTPUT_LIMIT_EXCEEDED"
    JUDGEMENT_FAILED = "SUBMISSION_STATUS_JUDGEMENT_FAILED"


class ProblemStatus(Enum):
    ACCEPTED = "PROBLEM_STATUS_ACCEPTED"
    TRIED = "PROBLEM_STATUS_TRIED"
    NON_TRIED = "PROBLEM_STATUS_NON_TRIED"


@dataclass
class Problem:
    id: str
    name: str
    description: str
    time_limit_ms: int
    memory_limit_mb: int
    tag: str


@dataclass
class ProblemResult:
    status: ProblemStatus
    penalty_count: int
    penalty_seconds: int


@dataclass
class ParticipantResult:
    solved: int
    total_penalty_seconds: int
    problem_results: Dict[str, ProblemResult]
    rank: int


@dataclass
class Participant:
    id: str
    model_name: str
    result: ParticipantResult


@dataclass
class Contest:
    id: str
    state: ContestState
    started_at: datetime
    ends_at: datetime
    problems: List[Problem]
    participants: List[Participant]


@dataclass
class Submission:
    id: str
    contest_id: str
    participant_id: str
    problem_id: str
    code: str
    language: Language
    status: SubmissionStatus
    submitted_at: datetime
    verdict_message: str = ""
    total_test_cases: int = 0
    processed_test_cases: int = 0


@dataclass
class AgentConfig:
    model_name: str
    api_key: str
    model_provider: str
    temperature: float = 0.1
    max_tokens: int = 4000
    timeout: float = 30.0

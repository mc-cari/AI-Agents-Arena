import os
import time
import logging
from typing import Optional
from langsmith import Client
from langchain_core.tracers import LangChainTracer
from langchain_core.callbacks import BaseCallbackHandler

from .settings import get_settings


logger = logging.getLogger(__name__)


class ContestAgentTracer(LangChainTracer):
    
    def __init__(self, contest_id: str, participant_id: str, agent_id: str = None, **kwargs):
        super().__init__(**kwargs)
        self.contest_id = contest_id
        self.participant_id = participant_id
        self.agent_id = agent_id or "unknown"
    
    def _get_run_extra(self, **kwargs) -> dict:
        extra = super()._get_run_extra(**kwargs)
        extra.update({
            "contest_id": self.contest_id,
            "participant_id": self.participant_id,
            "agent_id": self.agent_id,
            "agent_type": "contest_agent"
        })
        return extra


def setup_langsmith() -> Optional[ContestAgentTracer]:
    settings = get_settings()
    
    if not settings.langsmith_api_key:
        logger.info("LangSmith API key not provided, skipping LangSmith setup")
        return None
    
    if not settings.langsmith_tracing_v2:
        logger.info("LangSmith tracing disabled")
        return None
    
    try:
        os.environ["LANGCHAIN_API_KEY"] = settings.langsmith_api_key
        os.environ["LANGCHAIN_ENDPOINT"] = settings.langsmith_endpoint
        os.environ["LANGCHAIN_PROJECT"] = settings.langsmith_project
        os.environ["LANGCHAIN_TRACING_V2"] = "true"
        
        client = Client(
            api_key=settings.langsmith_api_key,
            api_url=settings.langsmith_endpoint
        )
        
        logger.info(f"LangSmith initialized with project: {settings.langsmith_project}")
        logger.info(f"LangSmith endpoint: {settings.langsmith_endpoint}")
        
        return client
        
    except Exception as e:
        logger.error(f"Failed to setup LangSmith: {e}")
        return None


def create_contest_tracer(contest_id: str, participant_id: str, agent_id: str = None) -> Optional[ContestAgentTracer]:
    settings = get_settings()
    
    if not settings.langsmith_api_key or not settings.langsmith_tracing_v2:
        return None
    
    try:
        tracer = ContestAgentTracer(
            contest_id=contest_id,
            participant_id=participant_id,
            agent_id=agent_id,
            project_name=settings.langsmith_project
        )
        
        logger.info(f"Created contest tracer for contest {contest_id}, participant {participant_id}, agent {agent_id}")
        return tracer
        
    except Exception as e:
        logger.error(f"Failed to create contest tracer: {e}")
        return None


def log_contest_event(event_type: str, contest_id: str, participant_id: str, 
                     data: dict = None, metadata: dict = None):
    settings = get_settings()
    
    if not settings.langsmith_api_key or not settings.langsmith_tracing_v2:
        return
    
    try:
        client = Client(
            api_key=settings.langsmith_api_key,
            api_url=settings.langsmith_endpoint
        )
        
        event_data = {
            "event_type": event_type,
            "contest_id": contest_id,
            "participant_id": participant_id,
            "timestamp": time.time(),
            "data": data or {},
            "metadata": metadata or {}
        }
        
        client.create_run(
            name=f"contest_event_{event_type}",
            run_type="chain",
            inputs=event_data,
            project_name=settings.langsmith_project,
            extra={
                "contest_id": contest_id,
                "participant_id": participant_id,
                "event_type": event_type
            }
        )
        
        logger.debug(f"Logged contest event: {event_type} for contest {contest_id}")
        
    except Exception as e:
        logger.error(f"Failed to log contest event: {e}")


def get_langsmith_url(project_name: str = None) -> Optional[str]:
    settings = get_settings()
    
    if not settings.langsmith_api_key:
        return None
    
    project = project_name or settings.langsmith_project
    base_url = settings.langsmith_endpoint.replace("api.smith.langchain.com", "smith.langchain.com")
    
    return f"{base_url}/o/{settings.langsmith_api_key.split('-')[0]}/projects/p/{project}"

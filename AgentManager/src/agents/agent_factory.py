from typing import Optional, Any, Dict, List
from langchain_openai import ChatOpenAI
from langchain_anthropic import ChatAnthropic
from langchain_google_genai import ChatGoogleGenerativeAI
from langchain_core.language_models import BaseChatModel
import logging

from ..models import AgentConfig
from ..config import get_settings, MODEL_REGISTRY, get_available_models, get_provider_for_model
from ..workflows import ContestAgent


class LLMFactory:
  
    @staticmethod
    def get_available_models() -> List[str]:
        return get_available_models()
    
    @staticmethod
    def get_provider_for_model(model_name: str) -> str:
        return get_provider_for_model(model_name)
    
    @staticmethod
    def create_llm_from_model(model_name: str, api_key: Optional[str] = None, **kwargs) -> BaseChatModel:
        logger = logging.getLogger(__name__)
        settings = get_settings()
        
        try:
            provider = get_provider_for_model(model_name)
            
            if not api_key:
                if provider == "openai":
                    api_key = settings.openai_api_key
                elif provider == "anthropic":
                    api_key = settings.anthropic_api_key
                elif provider == "google":
                    api_key = settings.google_api_key
                
                if not api_key:
                    raise ValueError(f"API key not found for {provider}. Set {provider.upper()}_API_KEY environment variable.")
            
            temperature = kwargs.get("temperature", settings.temperature)
            max_tokens = kwargs.get("max_tokens", settings.max_tokens)
            timeout = kwargs.get("timeout", settings.timeout)
            
            if provider == "openai":
                return ChatOpenAI(
                    model=model_name,
                    api_key=api_key,
                    temperature=temperature,
                    max_tokens=max_tokens,
                    timeout=timeout
                )
            
            elif provider == "anthropic":
                return ChatAnthropic(
                    model=model_name,
                    api_key=api_key,
                    temperature=temperature,
                    max_tokens=max_tokens,
                    timeout=timeout
                )
            
            elif provider == "google":
                return ChatGoogleGenerativeAI(
                    model=model_name,
                    google_api_key=api_key,
                    temperature=temperature,
                    max_output_tokens=max_tokens
                )
            
            else:
                raise ValueError(f"Unsupported provider: {provider}")
        
        except Exception as e:
            logger.error(f"Failed to create LLM for model {model_name}: {e}")
            raise


class AgentManager:
    
    def __init__(self):
        self.logger = logging.getLogger(__name__)
        self.active_agents = {}
    
    def create_agent_from_model(self, model_name: str, client, contest_id: str, participant_id: str, agent_id: str = None, status_callback=None, **kwargs):
        settings = get_settings()
        
        try:
            llm = LLMFactory.create_llm_from_model(model_name, **kwargs)
            
            provider = get_provider_for_model(model_name)
            config = AgentConfig(
                model_name=model_name,
                api_key=kwargs.get("api_key", ""),
                model_provider=provider,
                temperature=kwargs.get("temperature", settings.temperature),
                max_tokens=kwargs.get("max_tokens", settings.max_tokens),
                timeout=kwargs.get("timeout", settings.timeout)
            )
            
            problems = kwargs.get("problems", [])
            agent = ContestAgent(config, client, llm, contest_id, participant_id, agent_id, status_callback, problems)
            
            agent_key = f"{contest_id}_{participant_id}"
            self.active_agents[agent_key] = agent
            
            self.logger.info(f"Created agent for {model_name} in contest {contest_id}")
            return agent
        
        except Exception as e:
            self.logger.error(f"Failed to create agent for model {model_name}: {e}")
            raise
    
    def get_agent(self, contest_id: str, participant_id: str) -> Optional[Any]:
        agent_key = f"{contest_id}_{participant_id}"
        return self.active_agents.get(agent_key)
    
    def remove_agent(self, contest_id: str, participant_id: str):
        agent_key = f"{contest_id}_{participant_id}"
        agent_key = f"{contest_id}_{participant_id}"
        if agent_key in self.active_agents:
            del self.active_agents[agent_key]
            self.logger.info(f"Removed agent for contest {contest_id}")
    
    def list_active_agents(self) -> list:
        return list(self.active_agents.keys())

import os
from typing import Optional
from pydantic import BaseModel, Field
from dotenv import load_dotenv


class Settings(BaseModel):
    
    openai_api_key: Optional[str] = Field(None)
    anthropic_api_key: Optional[str] = Field(None) 
    google_api_key: Optional[str] = Field(None)
    
    model_name: str = Field("gpt-4")
    
    temperature: float = Field(0.1)
    max_tokens: int = Field(4000)
    timeout: float = Field(30.0)
    
    contest_id: Optional[str] = Field(None)
    participant_id: Optional[str] = Field(None)
    
    grpc_server_host: str = Field("localhost")
    grpc_server_port: int = Field(50051)
    
    log_level: str = Field("INFO")
    log_format: str = Field("%(asctime)s - %(name)s - %(levelname)s - %(message)s")
    

    max_concurrent_agents: int = Field(10)
    agent_timeout: float = Field(300.0)
    
    def __init__(self, **kwargs):
        load_dotenv()
        
        env_data = {
            'openai_api_key': os.getenv('OPENAI_API_KEY'),
            'anthropic_api_key': os.getenv('ANTHROPIC_API_KEY'),
            'google_api_key': os.getenv('GOOGLE_API_KEY'),
            'model_name': os.getenv('MODEL_NAME', 'gpt-4'),
            'temperature': float(os.getenv('TEMPERATURE', '0.1')),
            'max_tokens': int(os.getenv('MAX_TOKENS', '4000')),
            'timeout': float(os.getenv('TIMEOUT', '30.0')),
            'contest_id': os.getenv('CONTEST_ID'),
            'participant_id': os.getenv('PARTICIPANT_ID'),
            'grpc_server_host': os.getenv('GRPC_SERVER_HOST', 'localhost'),
            'grpc_server_port': int(os.getenv('GRPC_SERVER_PORT', '50051')),
            'log_level': os.getenv('LOG_LEVEL', 'INFO'),
            'log_format': os.getenv('LOG_FORMAT', '%(asctime)s - %(name)s - %(levelname)s - %(message)s'),
            'max_concurrent_agents': int(os.getenv('MAX_CONCURRENT_AGENTS', '10')),
            'agent_timeout': float(os.getenv('AGENT_TIMEOUT', '300.0')),
        }
        
        env_data.update(kwargs)
        
        env_data = {k: v for k, v in env_data.items() if v is not None}
        
        super().__init__(**env_data)


_settings: Optional[Settings] = None


def get_settings() -> Settings:
    global _settings
    if _settings is None:
        _settings = Settings()
    return _settings


def reload_settings():
    global _settings
    _settings = Settings()
    return _settings

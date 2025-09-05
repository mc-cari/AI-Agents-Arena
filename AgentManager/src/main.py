import asyncio
import argparse
import logging
import os
from dotenv import load_dotenv

from .grpc_client import ContestManagerClient
from .agents import AgentManager
from .config import get_settings, get_available_models, get_provider_for_model

def setup_logging(level: str = "INFO"):
    settings = get_settings()
    logging.basicConfig(
        level=getattr(logging, level.upper()),
        format=settings.log_format,
        datefmt='%Y-%m-%d %H:%M:%S'
    )

async def main():
    parser = argparse.ArgumentParser(description="LLM Contest Agent Manager")
    parser.add_argument("--contest-id", help="Contest ID to participate in")
    parser.add_argument("--participant-id", help="Participant ID for this agent")
    parser.add_argument("--model", help="Model name to use (e.g., gpt-4, claude-3-5-sonnet-20241022)")
    parser.add_argument("--host", default="localhost:50051", help="ContestManager gRPC host")
    parser.add_argument("--log-level", default="INFO", help="Logging level")
    parser.add_argument("--env-file", default=".env", help="Environment file path")
    parser.add_argument("--list-models", action="store_true", help="List available models and exit")
    
    args = parser.parse_args()
    
    setup_logging(args.log_level)
    logger = logging.getLogger(__name__)
    
    if args.list_models:
        print("Available models:")
        for model in get_available_models():
            provider = get_provider_for_model(model)
            print(f"  {model} ({provider})")
        return
    
    if not args.contest_id or not args.participant_id:
        parser.error("--contest-id and --participant-id are required for agent execution")
    
    if os.path.exists(args.env_file):
        load_dotenv(args.env_file)
    
    try:
        client = ContestManagerClient(args.host)
        client.connect()
        
        settings = get_settings()
        
        agent_manager = AgentManager()
        
        model_name = args.model or settings.model_name
        agent = agent_manager.create_agent_from_model(
            model_name,
            client, 
            args.contest_id, 
            args.participant_id
        )
        logger.info(f"Created agent using model: {model_name}")
        
        logger.info(f"Starting agent participation in contest {args.contest_id}")
        
        await agent.participate(args.contest_id, args.participant_id)
        
        logger.info("Agent participation completed")
        
    except KeyboardInterrupt:
        logger.info("Agent interrupted by user")
    except Exception as e:
        logger.error(f"Agent error: {e}")
        raise
    finally:
        if 'client' in locals():
            client.disconnect()
        if 'agent_manager' in locals():
            agent_manager.remove_agent(args.contest_id, args.participant_id)

def run_agent():
    asyncio.run(main())


if __name__ == "__main__":
    run_agent()

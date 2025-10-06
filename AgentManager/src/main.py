import asyncio
import argparse
import logging
import os
import signal
from dotenv import load_dotenv

from .config import get_settings, get_available_models, get_provider_for_model

def setup_logging(level: str = "INFO"):
    settings = get_settings()
    logging.basicConfig(
        level=getattr(logging, level.upper()),
        format=settings.log_format,
        datefmt='%Y-%m-%d %H:%M:%S'
    )

async def run_server(port: int, max_workers: int):
    from .grpc_server import AgentManagerServer
    
    logger = logging.getLogger(__name__)
    logger.info(f"Starting AgentManager gRPC server on port {port}")
    
    server = AgentManagerServer(port=port, max_workers=max_workers)
    server.start()
    
    def signal_handler(sig, frame):
        logger.info("Received shutdown signal")
        server.stop()
    
    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)
    
    logger.info(f"AgentManager server listening on 0.0.0.0:{port}")
    logger.info("Press Ctrl+C to stop")
    
    try:
        server.wait_for_termination()
    except KeyboardInterrupt:
        logger.info("Server interrupted")
        server.stop()

async def main():
    parser = argparse.ArgumentParser(description="LLM Contest Agent Manager gRPC Server")
    parser.add_argument("--port", type=int, default=50052, help="Server port")
    parser.add_argument("--max-workers", type=int, default=10, help="Max worker threads")
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
    
    if os.path.exists(args.env_file):
        load_dotenv(args.env_file)
    
    await run_server(args.port, args.max_workers)

def run_server_main():
    asyncio.run(main())


if __name__ == "__main__":
    run_server_main()

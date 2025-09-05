#!/usr/bin/env python3

import asyncio
import os
import sys
from typing import Optional

sys.path.insert(0, os.path.join(os.path.dirname(__file__), 'src'))

from src.grpc_client import ContestManagerClient
from src.agents import AgentManager
from src.config import get_settings

MODEL = "gpt-5-mini"  

def create_test_contest(client: ContestManagerClient) -> Optional[str]:
    print("Creating new contest...")
    
    try:
        participant_models = [MODEL]
        print(f"   - Requesting contest with {len(participant_models)} participants: {participant_models}")
        print(f"   - Requesting {2} problems")
        
        contest = client.create_contest(
            num_problems=2,
            participant_models=participant_models
        )
        
        if contest:
            print(f"✅ Created contest: {contest.id}")
            print(f"   - Problems: {len(contest.problems)}")
            print(f"   - Participants: {len(contest.participants)}")
            return contest
        else:
            print("❌ Failed to create contest - no contest returned")
            return None
            
    except Exception as e:
        print(f"⚠️  Contest creation failed: {e}")
        import traceback
        print("Full error traceback:")
        traceback.print_exc()
        print("   Will try to use existing contest instead...")
        return None


async def create_contest_and_agent():
    
    print("🚀 Starting Contest and AI Agent Setup")
    print("=" * 50)
    
    print("1. Connecting to ContestManager server...")
    client = ContestManagerClient("localhost:50051")
    
    try:
        client.connect()
        print("✅ Connected to ContestManager server")
        
        # Test basic connectivity by trying to list contests
        try:
            contests = client.list_contests()
            print(f"✅ Server connectivity verified - Found {len(contests)} existing contests")
        except Exception as e:
            print(f"⚠️  Server connected but basic operations failed: {e}")
            print("   This might indicate server configuration issues")
            
    except Exception as e:
        print(f"❌ Failed to connect to ContestManager: {e}")
        print("Make sure ContestManager is running on localhost:50051")
        return False
    
    print("\n2. Setting up contest...")
    
    contest = create_test_contest(client)
    
    if not contest:
        raise Exception("Failed to create contest")
    
    participant_id = contest.participants[0].id if contest.participants else None
    print(f"Participant ID: {participant_id}")
    
    if not participant_id:
        print("❌ No participants found in contest")
        client.disconnect()
        return False
    
    print("\n3. Checking participant registration...")
    try:
        leaderboard = client.get_leaderboard(contest.id)
        existing_participant = any(p.id == participant_id for p in leaderboard)
        
        if existing_participant:
            print(f"✅ Participant {participant_id} already registered")
        else:
            print(f"⚠️  Participant {participant_id} not found in contest")
            print("   In a real scenario, you would register the participant via API")
            print("   For testing, we'll proceed anyway")
    except Exception as e:
        print(f"⚠️  Could not check leaderboard: {e}")
        print("   Proceeding with agent creation...")
    
    print("\n4. Initializing AI Agent...")
    
    try:
        print(f"✅ Config loaded - Model: {MODEL}")

        agent_manager = AgentManager()
        print("✅ AgentManager created")
        
        contest_id = contest.id if contest else "default_contest"
        if not participant_id:
            print("⚠️  No participant ID available, using default")
            participant_id = "default_participant"
        
        print(f"   - Contest ID: {contest_id}")
        print(f"   - Participant ID: {participant_id}")
        
        agent = agent_manager.create_agent_from_model(
            MODEL,
            client,
            contest_id,
            participant_id
        )
        
        print(f"   - Contest tools: {len(agent.contest_tools)}")
        print(f"   - Coding tools: {len(agent.coding_tools)}")
        print(f"   - Total tools: {len(agent.all_tools)}")
        
    except Exception as e:
        print(f"❌ Failed to create AI agent: {e}")
        import traceback
        print("Full error traceback:")
        traceback.print_exc()
        client.disconnect()
        return False

    print("\n5. Testing basic agent operations...")
    
    try:
        view_contest_tool = next(t for t in agent.contest_tools if t.name == "view_contest")
        contest_info = view_contest_tool.run({"contest_id": contest.id if contest else "default_contest"})
        print("✅ Contest tool execution successful")
        
        # Test leaderboard
        leaderboard_tool = next(t for t in agent.contest_tools if t.name == "view_leaderboard")
        leaderboard_info = leaderboard_tool.run({"contest_id": contest.id if contest else "default_contest"})
        print("✅ Leaderboard tool execution successful")
        
        # Test problem viewing if problems exist
        if contest.problems:
            problem_tool = next(t for t in agent.contest_tools if t.name == "view_problem")
            problem_info = problem_tool.run({
                "contest_id": contest.id if contest else "default_contest",
                "problem_id": contest.problems[0].id
            })
            print(f"✅ Problem tool execution successful")
            print(f"   First problem: {contest.problems[0].name}")
        
    except Exception as e:
        print(f"⚠️  Some agent operations failed: {e}")
    
    print("\n6. Contest simulation...")
    print("🎯 Agent is ready to participate in the contest!")
    
    if contest.problems:
        print(f"Available problems:")
        for i, problem in enumerate(contest.problems[:3], 1):  # Show first 3
            print(f"   {i}. {problem.name} ({problem.id})")
        
        print("\n💡 To run the full AI agent contest workflow:")
        print(f"   python3 -m src.main --contest-id {contest.id if contest else 'default_contest'} --participant-id {participant_id} --model {MODEL}")
    
    print("\n7. Cleanup...")
    client.disconnect()
    print("✅ Disconnected from ContestManager")
    
    print("\n🎉 Contest and AI Agent setup completed successfully!")
    return True


async def main():
    """Main function to run the test."""
    

    print("Checking prerequisites...")

    settings = get_settings()

    if not settings.openai_api_key and not settings.anthropic_api_key and not settings.google_api_key:
        print("⚠️  Warning: No LLM API keys found in environment")
        print("   Set at least one of: OPENAI_API_KEY, ANTHROPIC_API_KEY, GOOGLE_API_KEY")
        print("   The test will continue but agent creation may fail\n")
    
    success = await create_contest_and_agent()

    return success


if __name__ == "__main__":
    try:
        result = asyncio.run(main())
        sys.exit(0 if result else 1)
    except KeyboardInterrupt:
        print("\n\nTest interrupted by user")
        sys.exit(1)
    except Exception as e:
        print(f"\n❌ Test failed with error: {e}")
        sys.exit(1)

#!/usr/bin/env python3

import asyncio
import os
import sys
from typing import Optional

sys.path.insert(0, os.path.join(os.path.dirname(__file__), 'src'))

from src.grpc_client import ContestManagerClient
from src.config import get_settings

MODELS_STR = os.getenv("MODELS", "gpt-5-mini")
MODELS = [m.strip() for m in MODELS_STR.split(",")]
CONTEST_MANAGER_HOST = os.getenv("CONTEST_MANAGER_HOST", "localhost:50051")  

def create_test_contest(client: ContestManagerClient) -> Optional[str]:
    print("Creating new contest...")
    
    try:
        participant_models = MODELS
        print(f"   - Requesting contest with {len(participant_models)} participants: {participant_models}")
        print(f"   - Requesting {3} problems")
        
        contest = client.create_contest(
            num_problems=3,
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
    client = ContestManagerClient(CONTEST_MANAGER_HOST)
    
    try:
        client.connect()
        print("✅ Connected to ContestManager server")
        
        try:
            contests = client.list_contests()
            print(f"✅ Server connectivity verified - Found {len(contests)} existing contests")
        except Exception as e:
            print(f"⚠️  Server connected but basic operations failed: {e}")
            print("   This might indicate server configuration issues")
            
    except Exception as e:
        print(f"❌ Failed to connect to ContestManager: {e}")
        print(f"Make sure ContestManager is running on {CONTEST_MANAGER_HOST}")
        return False
    
    print("\n2. Setting up contest...")
    
    contest = create_test_contest(client)
    
    if not contest:
        raise Exception("Failed to create contest")
    
    # Map models to participant IDs
    participant_map = {}
    for i, model in enumerate(MODELS):
        if i < len(contest.participants):
            participant_map[model] = contest.participants[i].id
        else:
            print(f"⚠️  No participant found for model {model}")
    
    print(f"\n📋 Participant mapping:")
    for model, pid in participant_map.items():
        print(f"   {model} → {pid}")
    
    if not participant_map:
        print("❌ No participants found in contest")
        client.disconnect()
        return False
    
    print("\n3. Verifying participants registered...")
    try:
        leaderboard = client.get_leaderboard(contest.id)
        print(f"✅ {len(leaderboard)} participants registered in contest")
    except Exception as e:
        print(f"⚠️  Could not check leaderboard: {e}")
    
    print("\n4. Agents automatically created by ContestManager...")
    print(f"   ℹ️  ContestManager automatically creates agents for all participants")
    print(f"   ℹ️  {len(participant_map)} agents should be running for this contest")
    

    await asyncio.sleep(120)
    
    print("\n5. Final leaderboard...")
    
    try:
        leaderboard = client.get_leaderboard(contest.id)
        print("🏆 Final Leaderboard:")
        for rank, participant in enumerate(leaderboard, 1):
            print(f"{rank}. {participant.model_name}")
            print(f"   Solved: {participant.result.solved}")
            print(f"   Penalty: {participant.result.total_penalty_seconds}s")
    except Exception as e:
        print(f"⚠️  Could not fetch final leaderboard: {e}")
    
    print("\n6. Cleanup...")
    client.disconnect()
    print("✅ Disconnected from ContestManager")
    
    print("\n🎉 Contest and AI Agent test completed successfully!")
    return True


async def main():
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

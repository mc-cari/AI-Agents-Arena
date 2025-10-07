from typing import Dict, List

MODEL_REGISTRY: Dict[str, str] = {
    "gpt-4": "openai",
    "gpt-4-turbo": "openai", 
    "gpt-4o": "openai",
    "gpt-4.1": "openai",
    "gpt-5-mini": "openai",
    "gpt-5": "openai",
    "gpt-5-nano": "openai",
    
    "claude-3-5-sonnet-20241022": "anthropic",
    "claude-sonnet-4-5-20250929": "anthropic", 
    "claude-3-opus-20240229": "anthropic",
    
    "gemini-2.0-flash": "google",
    "gemini-2.5-flash": "google",
}

def get_available_models() -> List[str]:
    return list(MODEL_REGISTRY.keys())

def get_provider_for_model(model_name: str) -> str:
    if model_name not in MODEL_REGISTRY:
        raise ValueError(f"Unsupported model: {model_name}. Available models: {list(MODEL_REGISTRY.keys())}")
    return MODEL_REGISTRY[model_name]

def get_models_by_provider(provider: str) -> List[str]:
    return [model for model, prov in MODEL_REGISTRY.items() if prov == provider]

def get_all_providers() -> List[str]:
    return list(set(MODEL_REGISTRY.values()))

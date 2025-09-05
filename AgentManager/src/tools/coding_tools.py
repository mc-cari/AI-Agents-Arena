from langchain_core.tools import BaseTool
from typing import Type
from pydantic import BaseModel, Field
import re


class AnalyzeProblemInput(BaseModel):
    """Input for problem analysis."""
    problem_description: str = Field(description="The problem description")


class AnalyzeProblemTool(BaseTool):
    """Tool to analyze programming problems and suggest approaches."""
    
    name: str = "analyze_problem"
    description: str = "Analyze a programming problem and suggest solution approaches"
    args_schema: Type[BaseModel] = AnalyzeProblemInput
    
    def _run(self, problem_description: str) -> str:
        """Analyze a problem and suggest approaches."""
        analysis = "Problem Analysis:\n"
        analysis += "================\n\n"
        
        # Basic keyword detection for common problem types
        desc_lower = problem_description.lower()
        
        suggested_approaches = []
        
        if any(word in desc_lower for word in ['sort', 'sorted', 'order']):
            suggested_approaches.append("Sorting algorithm")
        
        if any(word in desc_lower for word in ['graph', 'tree', 'node', 'edge']):
            suggested_approaches.append("Graph/Tree traversal (DFS/BFS)")
        
        if any(word in desc_lower for word in ['dynamic', 'optimal', 'minimum', 'maximum']):
            suggested_approaches.append("Dynamic Programming")
        
        if any(word in desc_lower for word in ['substring', 'string', 'character']):
            suggested_approaches.append("String manipulation")
        
        if any(word in desc_lower for word in ['array', 'list', 'sequence']):
            suggested_approaches.append("Array processing")
        
        if any(word in desc_lower for word in ['binary', 'search', 'find']):
            suggested_approaches.append("Binary search")
        
        if any(word in desc_lower for word in ['greedy', 'choose', 'select']):
            suggested_approaches.append("Greedy algorithm")
        
        if not suggested_approaches:
            suggested_approaches.append("Brute force approach")
        
        analysis += "Suggested approaches:\n"
        for i, approach in enumerate(suggested_approaches, 1):
            analysis += f"{i}. {approach}\n"
        
        analysis += "\nConsiderations:\n"
        analysis += "- Read the input format carefully\n"
        analysis += "- Consider edge cases\n"
        analysis += "- Check time and memory constraints\n"
        analysis += "- Test with sample inputs\n"
        
        return analysis


class OptimizeCodeInput(BaseModel):
    """Input for code optimization."""
    code: str = Field(description="The code to optimize")
    language: str = Field(description="Programming language")
    constraints: str = Field(default="", description="Performance constraints")


class OptimizeCodeTool(BaseTool):
    """Tool to suggest code optimizations."""
    
    name: str = "optimize_code"
    description: str = "Analyze code and suggest optimizations"
    args_schema: Type[BaseModel] = OptimizeCodeInput
    
    def _run(self, code: str, language: str, constraints: str = "") -> str:
        """Optimize code."""
        suggestions = "Code Optimization Suggestions:\n"
        suggestions += "==============================\n\n"
        
        if language.lower() == "python":
            suggestions += self._analyze_python_code(code)
        elif language.lower() == "cpp":
            suggestions += self._analyze_cpp_code(code)
        
        return suggestions
    
    def _analyze_python_code(self, code: str) -> str:
        """Analyze Python code for optimizations."""
        suggestions = []
        
        if "for i in range(len(" in code:
            suggestions.append("Consider using enumerate() instead of range(len())")
        
        if ".append(" in code and "for" in code:
            suggestions.append("Consider using list comprehension")
        
        if "import *" in code:
            suggestions.append("Avoid wildcard imports, import specific functions")
        
        if not suggestions:
            suggestions.append("Code looks well-optimized for Python")
        
        return "\n".join(f"- {s}" for s in suggestions)
    
    def _analyze_cpp_code(self, code: str) -> str:
        """Analyze C++ code for optimizations."""
        suggestions = []
        
        if "endl" in code:
            suggestions.append("Consider using '\\n' instead of endl for better performance")
        
        if "vector<" in code and "push_back" in code:
            suggestions.append("Consider reserving vector capacity if size is known")
        
        if "cin" in code and "cout" in code:
            suggestions.append("Consider using ios_base::sync_with_stdio(false) for faster I/O")
        
        if not suggestions:
            suggestions.append("Code looks well-optimized for C++")
        
        return "\n".join(f"- {s}" for s in suggestions)


def create_coding_tools() -> list[BaseTool]:
    """Create all coding-related tools."""
    return [
        AnalyzeProblemTool(),
        OptimizeCodeTool(),
    ]

package executors

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"contestmanager/internal/models"
)


type DirectExecutor struct {
	baseDir string
}

func NewDirectExecutor() *DirectExecutor {
	return &DirectExecutor{
		baseDir: "/tmp/code-execution",
	}
}

func (de *DirectExecutor) Execute(ctx context.Context, req *models.ExecutionRequest) (*models.ExecutionResult, error) {
	results, err := de.ExecuteCode(ctx, req)
	if err != nil {
		// Check if this is a compilation error
		if strings.Contains(err.Error(), "compilation failed") {
			return &models.ExecutionResult{
				JobID:           req.JobID,
				SubmissionID:    req.SubmissionID,
				Status:          models.SubmissionStatusCompilationError,
				VerdictMessage:  fmt.Sprintf("Compilation failed: %v", err),
				TestCaseResults: results,
			}, nil
		}
		
		return &models.ExecutionResult{
			JobID:           req.JobID,
			SubmissionID:    req.SubmissionID,
			Status:          models.SubmissionStatusJudgementFailed,
			VerdictMessage:  fmt.Sprintf("Execution failed: %v", err),
			TestCaseResults: results,
		}, nil
	}

	status := models.SubmissionStatusAccepted
	verdictMessage := "All test cases passed"
	
	for _, result := range results {
		switch result.Status {
		case models.TestCaseStatusTimeLimitExceeded:
			status = models.SubmissionStatusTimeLimitExceeded
			verdictMessage = fmt.Sprintf("Time Limit Exceeded on test case %d", result.TestOrder)
		case models.TestCaseStatusMemoryLimitExceeded:
			status = models.SubmissionStatusMemoryLimitExceeded
			verdictMessage = fmt.Sprintf("Memory Limit Exceeded on test case %d", result.TestOrder)
		case models.TestCaseStatusRuntimeError:
			status = models.SubmissionStatusRuntimeError
			verdictMessage = fmt.Sprintf("Runtime Error on test case %d: %s", result.TestOrder, result.ErrorMessage)
		case models.TestCaseStatusWrongAnswer:
			status = models.SubmissionStatusWrongAnswer
			verdictMessage = fmt.Sprintf("Wrong Answer on test case %d", result.TestOrder)
		case models.TestCaseStatusPresentationError:
			status = models.SubmissionStatusPresentationError
			verdictMessage = fmt.Sprintf("Presentation Error on test case %d", result.TestOrder)
		}
	}

	return &models.ExecutionResult{
		JobID:           req.JobID,
		SubmissionID:    req.SubmissionID,
		Status:          status,
		VerdictMessage:  verdictMessage,
		TestCaseResults: results,
	}, nil
}

func (de *DirectExecutor) ExecuteCode(ctx context.Context, req *models.ExecutionRequest) ([]models.TestCaseResult, error) {
	// Create a unique directory for this execution
	execDir := filepath.Join(de.baseDir, fmt.Sprintf("exec-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(execDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create execution directory: %w", err)
	}
	defer os.RemoveAll(execDir)

	var results []models.TestCaseResult
	var err error

	switch req.Language {
	case models.LanguageCPP:
		results, err = de.executeCpp(ctx, req, execDir)
	case models.LanguagePython:
		results, err = de.executePython(ctx, req, execDir)
	default:
		return nil, fmt.Errorf("unsupported language: %v", req.Language)
	}

	if err != nil {
		return nil, err
	}

	return results, nil
}

// executeCpp compiles and executes C++ code
func (de *DirectExecutor) executeCpp(ctx context.Context, req *models.ExecutionRequest, execDir string) ([]models.TestCaseResult, error) {
	// Write source code to file
	sourceFile := filepath.Join(execDir, "solution.cpp")
	if err := os.WriteFile(sourceFile, []byte(req.Code), 0644); err != nil {
		return nil, fmt.Errorf("failed to write source file: %w", err)
	}

	// Compile C++ code
	compiledFile := filepath.Join(execDir, "solution")
	compileCmd := exec.CommandContext(ctx, "g++", "-std=c++20", "-O2", "-o", compiledFile, sourceFile)
	compileCmd.Dir = execDir
	
	compileOutput, err := compileCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("compilation failed: %s", string(compileOutput))
	}

	// Execute test cases
	var results []models.TestCaseResult
	for _, testCase := range req.TestCases {
		result, err := de.executeTestCase(ctx, req, testCase, compiledFile, execDir)
		if err != nil {
			return nil, fmt.Errorf("failed to execute test case %d: %w", testCase.TestOrder, err)
		}
		results = append(results, result)
	}

	return results, nil
}

// executePython executes Python code
func (de *DirectExecutor) executePython(ctx context.Context, req *models.ExecutionRequest, execDir string) ([]models.TestCaseResult, error) {
	// Write source code to file
	sourceFile := filepath.Join(execDir, "solution.py")
	if err := os.WriteFile(sourceFile, []byte(req.Code), 0644); err != nil {
		return nil, fmt.Errorf("failed to write source file: %w", err)
	}

	// Execute test cases
	var results []models.TestCaseResult
	for _, testCase := range req.TestCases {
		result, err := de.executeTestCase(ctx, req, testCase, "python3", execDir, sourceFile)
		if err != nil {
			return nil, fmt.Errorf("failed to execute test case %d: %w", testCase.TestOrder, err)
		}
		results = append(results, result)
	}

	return results, nil
}

// executeTestCase executes a single test case
func (de *DirectExecutor) executeTestCase(ctx context.Context, req *models.ExecutionRequest, testCase models.TestCaseData, executable string, execDir string, args ...string) (models.TestCaseResult, error) {
	// Create timeout context
	timeout := time.Duration(req.TimeLimitMs) * time.Millisecond
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Create command
	cmd := exec.CommandContext(timeoutCtx, executable, args...)
	cmd.Dir = execDir

	// Set up input/output pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return models.TestCaseResult{}, fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	defer stdin.Close()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return models.TestCaseResult{}, fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	defer stdout.Close()

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return models.TestCaseResult{}, fmt.Errorf("failed to create stderr pipe: %w", err)
	}
	defer stderr.Close()

	// Start the process
	startTime := time.Now()
	if err := cmd.Start(); err != nil {
		return models.TestCaseResult{}, fmt.Errorf("failed to start process: %w", err)
	}

	// Send input to stdin
	go func() {
		defer stdin.Close()
		io.WriteString(stdin, testCase.Input)
	}()

	// Read output
	var output strings.Builder
	var stderrOutput strings.Builder
	
	// Read stdout and stderr concurrently
	outputDone := make(chan error, 1)
	stderrDone := make(chan error, 1)
	
	go func() {
		_, err := io.Copy(&output, stdout)
		outputDone <- err
	}()
	
	go func() {
		_, err := io.Copy(&stderrOutput, stderr)
		stderrDone <- err
	}()

	// Wait for process completion or timeout
	processDone := make(chan error, 1)
	go func() {
		processDone <- cmd.Wait()
	}()

	var exitCode int
	var timedOut bool
	var memoryUsed int32

	select {
	case err := <-processDone:
		if err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				exitCode = exitError.ExitCode()
			} else {
				return models.TestCaseResult{}, fmt.Errorf("process error: %w", err)
			}
		}
	case <-timeoutCtx.Done():
		timedOut = true
		// Kill the process
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		// Wait for process to actually terminate
		<-processDone
	}

	// Wait for output reading to complete
	<-outputDone
	<-stderrDone

	execTime := time.Since(startTime)
	actualTimeMs := int32(execTime.Milliseconds())

	// Check for timeout
	if timedOut {
		return models.TestCaseResult{
			TestOrder:       testCase.TestOrder,
			Status:          models.TestCaseStatusTimeLimitExceeded,
			ExpectedOutput:  testCase.ExpectedOutput,
			ExecutionTimeMs: actualTimeMs,
		}, nil
	}

	// Check for runtime error
	if exitCode != 0 {
		return models.TestCaseResult{
			TestOrder:       testCase.TestOrder,
			Status:          models.TestCaseStatusRuntimeError,
			ActualOutput:    output.String(),
			ExpectedOutput:  testCase.ExpectedOutput,
			ExecutionTimeMs: actualTimeMs,
			MemoryUsedKb:    memoryUsed,
			ErrorMessage:    fmt.Sprintf("Process exited with code %d. Stderr: %s", exitCode, stderrOutput.String()),
		}, nil
	}


	status := models.TestCaseStatusPassed
	actualOutput := strings.TrimSpace(output.String())
	expectedOutput := strings.TrimSpace(testCase.ExpectedOutput)
	
	if actualOutput != expectedOutput {
		if de.compareOutputIgnoreWhitespace(actualOutput, expectedOutput) {
			status = models.TestCaseStatusPresentationError
			fmt.Printf("Status: Presentation Error (whitespace difference)\n")
		} else {
			status = models.TestCaseStatusWrongAnswer
			fmt.Printf("Status: Wrong Answer\n")
		}
	} else {
		fmt.Printf("Status: Passed\n")
	}
	

	return models.TestCaseResult{
		TestOrder:       testCase.TestOrder,
		Status:          status,
		ActualOutput:    actualOutput,
		ExpectedOutput:  expectedOutput,
		ExecutionTimeMs: actualTimeMs,
		MemoryUsedKb:    memoryUsed,
	}, nil
}

// compareOutputIgnoreWhitespace compares outputs ignoring whitespace differences
func (de *DirectExecutor) compareOutputIgnoreWhitespace(actual, expected string) bool {
	actualClean := strings.ReplaceAll(strings.ReplaceAll(actual, " ", ""), "\n", "")
	expectedClean := strings.ReplaceAll(strings.ReplaceAll(expected, " ", ""), "\n", "")
	return actualClean == expectedClean
}

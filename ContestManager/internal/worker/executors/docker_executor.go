package executors

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"contestmanager/internal/models"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

type DockerExecutor struct {
	client *client.Client
}

func NewDockerExecutor() *DockerExecutor {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Fatalf("Failed to create Docker client: %v", err)
	}

	executor := &DockerExecutor{
		client: cli,
	}
	
	executor.prePullImages()
	
	return executor
}

func (de *DockerExecutor) prePullImages() {
	images := []string{
		"gcc:latest",
		"python:3.13-slim-bullseye", 
		"alpine:latest",
	}
	
	for _, imageName := range images {
		log.Printf("Pre-pulling image: %s", imageName)
		_, err := de.client.ImagePull(context.Background(), imageName, image.PullOptions{})
		if err != nil {
			log.Printf("Failed to pre-pull %s: %v", imageName, err)
		}
	}
}

func (de *DockerExecutor) Execute(ctx context.Context, req *models.ExecutionRequest) (*models.ExecutionResult, error) {
	log.Printf("Executing %v code with %d test cases", req.Language, len(req.TestCases))

	tempDir, err := os.MkdirTemp("", "code-execution-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	codeFile, err := de.writeCodeToFile(tempDir, req.Code, req.Language)
	if err != nil {
		return nil, fmt.Errorf("failed to write code to file: %w", err)
	}

	_, compileOutput, err := de.compile(ctx, tempDir, codeFile, req.Language)
	if err != nil {
		return &models.ExecutionResult{
			JobID:          req.JobID,
			SubmissionID:   req.SubmissionID,
			Status:         models.SubmissionStatusCompilationError,
			VerdictMessage: "Compilation failed",
			CompilerOutput: compileOutput,
			ProcessedAt:    time.Now(),
		}, nil
	}

	results := make([]models.TestCaseResult, len(req.TestCases))
	passedCount := int32(0)
	totalExecTime := int32(0)
	maxMemory := int32(0)

	for i, testCase := range req.TestCases {
		result, err := de.executeTestCase(ctx, tempDir, testCase, req)
		if err != nil {
			return nil, fmt.Errorf("failed to execute test case %d: %w", i+1, err)
		}

		results[i] = *result
		
		if result.Status == models.TestCaseStatusPassed {
			passedCount++
		}
		
		totalExecTime += result.ExecutionTimeMs
		if result.MemoryUsedKb > maxMemory {
			maxMemory = result.MemoryUsedKb
		}

		if result.Status != models.TestCaseStatusPassed {
			for j := i + 1; j < len(req.TestCases); j++ {
				results[j] = models.TestCaseResult{
					TestOrder:      req.TestCases[j].TestOrder,
					Status:         "NOT_EXECUTED",
					ExpectedOutput: req.TestCases[j].ExpectedOutput,
				}
			}
			break
		}
	}

	status := models.SubmissionStatusAccepted
	verdictMessage := "Accepted"
	
	if passedCount < int32(len(req.TestCases)) {
		for _, result := range results {
			if result.Status != models.TestCaseStatusPassed {
				switch result.Status {
				case models.TestCaseStatusWrongAnswer:
					status = models.SubmissionStatusWrongAnswer
					verdictMessage = fmt.Sprintf("Wrong Answer on test case %d", result.TestOrder)
				case models.TestCaseStatusTimeLimitExceeded:
					status = models.SubmissionStatusTimeLimitExceeded
					verdictMessage = fmt.Sprintf("Time Limit Exceeded on test case %d", result.TestOrder)
				case models.TestCaseStatusMemoryLimitExceeded:
					status = models.SubmissionStatusMemoryLimitExceeded
					verdictMessage = fmt.Sprintf("Memory Limit Exceeded on test case %d", result.TestOrder)
				case models.TestCaseStatusRuntimeError:
					status = models.SubmissionStatusRuntimeError
					verdictMessage = fmt.Sprintf("Runtime Error on test case %d", result.TestOrder)
				case models.TestCaseStatusPresentationError:
					status = models.SubmissionStatusPresentationError
					verdictMessage = fmt.Sprintf("Presentation Error on test case %d", result.TestOrder)
				default:
					status = models.SubmissionStatusJudgementFailed
					verdictMessage = fmt.Sprintf("Unknown error on test case %d", result.TestOrder)
				}
				break
			}
		}
	}

	return &models.ExecutionResult{
		JobID:           req.JobID,
		SubmissionID:    req.SubmissionID,
		Status:          status,
		VerdictMessage:  verdictMessage,
		TestCaseResults: results,
		TotalTestCases:  int32(len(req.TestCases)),
		PassedTestCases: passedCount,
		ExecutionTimeMs: totalExecTime,
		MemoryUsedKb:    maxMemory,
		CompilerOutput:  compileOutput,
		ProcessedAt:     time.Now(),
	}, nil
}

func (de *DockerExecutor) writeCodeToFile(tempDir, code string, language models.Language) (string, error) {
	var filename string
	switch language {
	case models.LanguageCPP:
		filename = "solution.cpp"
	case models.LanguagePython:
		filename = "solution.py"
	default:
		return "", fmt.Errorf("unsupported language: %v", language)
	}

	filePath := filepath.Join(tempDir, filename)
	err := os.WriteFile(filePath, []byte(code), 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write code file: %w", err)
	}

	return filePath, nil
}

func (de *DockerExecutor) compile(ctx context.Context, tempDir, codeFile string, language models.Language) (string, string, error) {
	if language == models.LanguagePython {
		return codeFile, "", nil
	}

		if language == models.LanguageCPP {
		containerConfig := &container.Config{
			Image:        "gcc:latest",
			Cmd:          []string{"g++", "-o", "/code/solution", "/code/solution.cpp", "-std=c++20", "-O2"},
			WorkingDir:   "/code",
			AttachStdout: true,
			AttachStderr: true,
		}

		hostConfig := &container.HostConfig{
			Mounts: []mount.Mount{
				{
					Type:   mount.TypeBind,
					Source: tempDir,
					Target: "/code",
				},
			},
		Resources: container.Resources{
			Memory: 1024 * 1024 * 1024, 
		},
	}

		containerName := fmt.Sprintf("compile-%d", time.Now().UnixNano())
		resp, err := de.client.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, containerName)
		if err != nil {
			return "", "", fmt.Errorf("failed to create compilation container: %w", err)
		}
		defer de.client.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

		if err := de.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
			return "", "", fmt.Errorf("failed to start compilation container: %w", err)
		}

		statusCh, errCh := de.client.ContainerWait(ctx, resp.ID, container.WaitConditionNotRunning)
		select {
		case err := <-errCh:
			if err != nil {
				return "", "", fmt.Errorf("compilation container error: %w", err)
			}
		case status := <-statusCh:
			if status.StatusCode != 0 {
				logs, err := de.client.ContainerLogs(ctx, resp.ID, container.LogsOptions{
					ShowStdout: true,
					ShowStderr: true,
				})
				if err != nil {
					return "", "", fmt.Errorf("compilation failed and couldn't get logs: %w", err)
				}
				defer logs.Close()

				output, _ := io.ReadAll(logs)
				compilerOutput := string(output)
				if len(compilerOutput) > 2048 {
					compilerOutput = compilerOutput[:2048] + "... (output truncated)"
				}
				return "", compilerOutput, fmt.Errorf("compilation failed")
			}
		}

		executablePath := filepath.Join(tempDir, "solution")
		return executablePath, "", nil
	}

	return "", "", fmt.Errorf("unsupported language for compilation: %v", language)
}

func (de *DockerExecutor) executeTestCase(ctx context.Context, tempDir string, testCase models.TestCaseData, req *models.ExecutionRequest) (*models.TestCaseResult, error) {
	log.Printf("Executing test case %d with %d bytes input", testCase.TestOrder, len(testCase.Input))
	
	testCaseDir := filepath.Join(tempDir, fmt.Sprintf("test_%d", testCase.TestOrder))
	err := os.MkdirAll(testCaseDir, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create test case directory: %w", err)
	}
	defer os.RemoveAll(testCaseDir)
	
	solutionFile := filepath.Join(tempDir, "solution")
	if req.Language == models.LanguageCPP {
		destSolution := filepath.Join(testCaseDir, "solution")
		err = copyFile(solutionFile, destSolution)
		if err != nil {
			return nil, fmt.Errorf("failed to copy solution binary: %w", err)
		}
	} else if req.Language == models.LanguagePython {
		sourceScript := filepath.Join(tempDir, "solution.py")
		destScript := filepath.Join(testCaseDir, "solution.py")
		err = copyFile(sourceScript, destScript)
		if err != nil {
			return nil, fmt.Errorf("failed to copy Python script: %w", err)
		}
	}
	
	inputFile := filepath.Join(testCaseDir, "input.txt")
	err = os.WriteFile(inputFile, []byte(testCase.Input), 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to write input file: %w", err)
	}

	var cmd []string
	var image string
	
	switch req.Language {
	case models.LanguageCPP:
		image = "gcc:latest"
		cmd = []string{"/code/solution"}
	case models.LanguagePython:
		image = "python:3.13-slim-bullseye"
		cmd = []string{"python", "/code/solution.py"}
	default:
		return nil, fmt.Errorf("unsupported language: %v", req.Language)
	}

	containerConfig := &container.Config{
		Image:        image,
		Cmd:          cmd,
		WorkingDir:   "/code",
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		StdinOnce:    true,
		OpenStdin:    true,
	}

	hostConfig := &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: testCaseDir,
				Target: "/code",
			},
		},
		Resources: container.Resources{
			Memory: int64(req.MemoryLimitMb) * 1024 * 1024,
		},
		NetworkMode: "none",
		Privileged: false,
		ReadonlyRootfs: false,
		SecurityOpt: []string{"no-new-privileges"},
		CapAdd: []string{"SYS_PTRACE"},
	}

	containerName := fmt.Sprintf("exec-%s-%d-%d", req.SubmissionID, testCase.TestOrder, time.Now().UnixNano())
	resp, err := de.client.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, containerName)
	if err != nil {
		return nil, fmt.Errorf("failed to create execution container: %w", err)
	}
	defer de.client.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})

	if err := de.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("failed to start execution container: %w", err)
	}

	attachResp, err := de.client.ContainerAttach(ctx, resp.ID, container.AttachOptions{
		Stream: true,
		Stdin:  true,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to attach to container: %w", err)
	}
	defer attachResp.Close()

	go func() {
		defer attachResp.CloseWrite()
		attachResp.Conn.Write([]byte(testCase.Input))
	}()

	totalTimeout := time.Duration(req.TimeLimitMs+50) * time.Millisecond
	log.Printf("Setting timeout for test case %d: %dms (limit: %dms + 50ms buffer)", testCase.TestOrder, totalTimeout.Milliseconds(), req.TimeLimitMs)
	timeoutCtx, cancel := context.WithTimeout(ctx, totalTimeout)
	defer cancel()

	startTime := time.Now()
	statusCh, errCh := de.client.ContainerWait(timeoutCtx, resp.ID, container.WaitConditionNotRunning)
	
	var exitCode int64
	var timedOut bool
	
	select {
	case err := <-errCh:
		if err != nil {
			if err == context.DeadlineExceeded || timeoutCtx.Err() == context.DeadlineExceeded {
				timedOut = true
			} else {
				return nil, fmt.Errorf("container wait error: %w", err)
			}
		}
	case status := <-statusCh:
		exitCode = status.StatusCode
	case <-timeoutCtx.Done():
		timedOut = true
		log.Printf("Timeout reached for test case %d, killing container %s", testCase.TestOrder, resp.ID)
		err := de.client.ContainerKill(ctx, resp.ID, "SIGKILL")
		if err != nil {
			log.Printf("Failed to kill container %s: %v", resp.ID, err)
		} else {
			log.Printf("Successfully killed container %s", resp.ID)
		}
	}

	execTime := time.Since(startTime)
	actualTimeMs := int32(execTime.Milliseconds())
	
	log.Printf("Test case %d executed in %dms (limit: %dms)", testCase.TestOrder, actualTimeMs, req.TimeLimitMs)
	
	if timedOut {
		return &models.TestCaseResult{
			TestOrder:       testCase.TestOrder,
			Status:          models.TestCaseStatusTimeLimitExceeded,
			ExpectedOutput:  testCase.ExpectedOutput,
			ExecutionTimeMs: actualTimeMs,
		}, nil
	}

	stats, err := de.client.ContainerStats(ctx, resp.ID, false)
	if err != nil {
		log.Printf("Failed to get container stats: %v", err)
	}
	
	var memoryUsed int32 = 0
	if stats.Body != nil {
		defer stats.Body.Close()

		var statsData struct {
			MemoryStats struct {
				Usage int64 `json:"usage"`
			} `json:"memory_stats"`
		}
		
		if err := json.NewDecoder(stats.Body).Decode(&statsData); err == nil {
			memoryUsed = int32(statsData.MemoryStats.Usage / 1024)
		}
	}

	output, err := de.readContainerOutput(ctx, resp.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to read container output: %w", err)
	}

	if exitCode != 0 {
		return &models.TestCaseResult{
			TestOrder:       testCase.TestOrder,
			Status:          models.TestCaseStatusRuntimeError,
			ActualOutput:    output,
			ExpectedOutput:  testCase.ExpectedOutput,
			ExecutionTimeMs: actualTimeMs,
			MemoryUsedKb:    memoryUsed,
			ErrorMessage:    fmt.Sprintf("Process exited with code %d", exitCode),
		}, nil
	}

	if memoryUsed > req.MemoryLimitMb*1024 {
		return &models.TestCaseResult{
			TestOrder:       testCase.TestOrder,
			Status:          models.TestCaseStatusMemoryLimitExceeded,
			ActualOutput:    output,
			ExpectedOutput:  testCase.ExpectedOutput,
			ExecutionTimeMs: actualTimeMs,
			MemoryUsedKb:    memoryUsed,
		}, nil
	}

	status := models.TestCaseStatusPassed
	if !de.compareOutput(output, testCase.ExpectedOutput) {
		if de.compareOutputIgnoreWhitespace(output, testCase.ExpectedOutput) {
			status = models.TestCaseStatusPresentationError
		} else {
			status = models.TestCaseStatusWrongAnswer
		}
	}

	return &models.TestCaseResult{
		TestOrder:       testCase.TestOrder,
		Status:          status,
		ActualOutput:    output,
		ExpectedOutput:  testCase.ExpectedOutput,
		ExecutionTimeMs: actualTimeMs,
		MemoryUsedKb:    memoryUsed,
	}, nil
}

func (de *DockerExecutor) readContainerOutput(ctx context.Context, containerID string) (string, error) {
	logs, err := de.client.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: false,
	})
	if err != nil {
		return "", err
	}
	defer logs.Close()

	var output strings.Builder
	reader := bufio.NewReader(logs)
	
	for {
		header := make([]byte, 8)
		_, err := io.ReadFull(reader, header)
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		
		size := int(header[4])<<24 | int(header[5])<<16 | int(header[6])<<8 | int(header[7])
		
		payload := make([]byte, size)
		_, err = io.ReadFull(reader, payload)
		if err != nil {
			return "", err
		}
		
		output.Write(payload)
	}

	return strings.TrimSpace(output.String()), nil
}

func (de *DockerExecutor) compareOutput(actual, expected string) bool {
	return strings.TrimSpace(actual) == strings.TrimSpace(expected)
}

func (de *DockerExecutor) compareOutputIgnoreWhitespace(actual, expected string) bool {

	actualClean := strings.ReplaceAll(strings.ReplaceAll(actual, " ", ""), "\n", "")
	expectedClean := strings.ReplaceAll(strings.ReplaceAll(expected, " ", ""), "\n", "")
	return actualClean == expectedClean
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	
	return os.Chmod(dst, sourceInfo.Mode())
}

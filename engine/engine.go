// Package engine provides the implementation of Terragrunt IaC engine interface
package engine

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
	tgengine "github.com/gruntwork-io/terragrunt-engine-go/proto"
	"github.com/hashicorp/go-plugin"
	log "github.com/sirupsen/logrus"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
	"google.golang.org/grpc"
)

const (
	wgSize          = 2
	iacCommand      = "terraform"
	errorResultCode = 1
)

type TerraformEngine struct {
	tgengine.UnimplementedEngineServer
}

func (c *TerraformEngine) Init(req *tgengine.InitRequest, stream tgengine.Engine_InitServer) error {
	log.Info("Init Terraform engine")

	err := stream.Send(&tgengine.InitResponse{Stdout: "Terraform Initialization completed\n", Stderr: "", ResultCode: 0})
	if err != nil {
		return err
	}

	return nil
}

func (c *TerraformEngine) Run(req *tgengine.RunRequest, stream tgengine.Engine_RunServer) error {
	log.Infof("Run Terraform engine %v", req.GetWorkingDir())
	cmd := exec.Command(iacCommand, req.GetArgs()...)
	cmd.Dir = req.GetWorkingDir()

	env := make([]string, 0, len(req.GetEnvVars()))
	for key, value := range req.GetEnvVars() {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	cmd.Env = append(cmd.Env, env...)

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		sendError(stream, err)
		return err
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		sendError(stream, err)
		return err
	}

	if req.GetAllocatePseudoTty() {
		ptmx, err := pty.Start(cmd)
		if err != nil {
			log.Errorf("Error allocating pseudo-TTY: %v", err)
			return err
		}

		defer func() { _ = ptmx.Close() }()

		go func() {
			_, _ = io.Copy(ptmx, os.Stdin)
		}()
		go func() {
			_, _ = io.Copy(os.Stdout, ptmx)
		}()
		go func() {
			_, _ = io.Copy(os.Stderr, ptmx)
		}()
	} else {
		cmd.Stdin = os.Stdin
	}

	if err := cmd.Start(); err != nil {
		sendError(stream, err)
		return err
	}

	var wg sync.WaitGroup

	wg.Add(wgSize)

	go func() {
		// Ensure this goroutine signals completion when it returns
		defer wg.Done()

		// Create a reader that translates data from stdoutPipe into UTF-8 runes
		reader := transform.NewReader(stdoutPipe, unicode.UTF8.NewDecoder())
		// Wrap the reader in a buffered reader for efficient reading
		bufReader := bufio.NewReader(reader)

		for {
			// Read a single rune from the buffered reader
			char, _, err := bufReader.ReadRune()
			if err != nil {
				// If there's an error and it's not EOF, log it
				if !errors.Is(err, io.EOF) {
					log.Errorf("Error reading stdout: %v", err)
				}
				// Exit the loop on EOF or any other error
				break
			}

			// Stream the read character back to the client
			if err = stream.Send(&tgengine.RunResponse{Stdout: string(char)}); err != nil {
				// If streaming fails, log the error and exit
				log.Errorf("Error sending stdout: %v", err)
				return
			}
		}
	}()

	// Starts a goroutine that captures stderr output character by character,
	// applying UTF-8 decoding, and streams each character to the client.
	// Handles errors appropriately and signals completion via WaitGroup.
	// Terminates on EOF or transmission errors.
	go func() {
		defer wg.Done()

		reader := transform.NewReader(stderrPipe, unicode.UTF8.NewDecoder())
		bufReader := bufio.NewReader(reader)

		for {
			char, _, err := bufReader.ReadRune()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					log.Errorf("Error reading stderr: %v", err)
				}

				break
			}

			if err = stream.Send(&tgengine.RunResponse{Stderr: string(char)}); err != nil {
				log.Errorf("Error sending stderr: %v", err)
				return
			}
		}
	}()
	wg.Wait()

	resultCode := 0

	if err := cmd.Wait(); err != nil {
		var exitError *exec.ExitError
		if ok := errors.As(err, &exitError); ok {
			resultCode = exitError.ExitCode()
		} else {
			resultCode = 1
		}
	}

	if err := stream.Send(&tgengine.RunResponse{ResultCode: int32(resultCode)}); err != nil {
		return err
	}

	return nil
}

func sendError(stream tgengine.Engine_RunServer, err error) {
	if err = stream.Send(&tgengine.RunResponse{Stderr: fmt.Sprintf("%v", err), ResultCode: errorResultCode}); err != nil {
		log.Warnf("Error sending response: %v", err)
	}
}

func (c *TerraformEngine) Shutdown(req *tgengine.ShutdownRequest, stream tgengine.Engine_ShutdownServer) error {
	log.Info("Shutdown Terraform engine")

	err := stream.Send(&tgengine.ShutdownResponse{Stdout: "Terraform Shutdown completed\n", Stderr: "", ResultCode: 0})
	if err != nil {
		return err
	}

	return nil
}

// GRPCServer is used to register the Terraform Engine with the gRPC server
func (c *TerraformEngine) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	tgengine.RegisterEngineServer(s, c)
	return nil
}

// GRPCClient is used to create a client that connects to the Terraform Engine
func (c *TerraformEngine) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, client *grpc.ClientConn) (interface{}, error) {
	return tgengine.NewEngineClient(client), nil
}

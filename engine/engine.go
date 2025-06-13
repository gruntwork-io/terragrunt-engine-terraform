// Package engine implements a Terraform engine to be used with Terragrunt.
package engine

import (
	"bufio"
	"context"
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
	log.Infof("Run Terraform engine %v", req.WorkingDir)
	cmd := exec.Command(iacCommand, req.Args...)
	cmd.Dir = req.WorkingDir
	env := make([]string, 0, len(req.EnvVars))
	for key, value := range req.EnvVars {
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

	if req.AllocatePseudoTty {
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

	// 2 streams to send stdout and stderr
	wg.Add(wgSize)

	// Stream stdout
	go func() {
		defer wg.Done()
		reader := transform.NewReader(stdoutPipe, unicode.UTF8.NewDecoder())
		bufReader := bufio.NewReader(reader)
		for {
			char, _, err := bufReader.ReadRune()
			if err != nil {
				if err != io.EOF {
					log.Errorf("Error reading stdout: %v", err)
				}
				break
			}
			if err = stream.Send(&tgengine.RunResponse{Stdout: string(char)}); err != nil {
				log.Errorf("Error sending stdout: %v", err)
				return
			}
		}
	}()

	// Stream stderr
	go func() {
		defer wg.Done()
		reader := transform.NewReader(stderrPipe, unicode.UTF8.NewDecoder())
		bufReader := bufio.NewReader(reader)
		for {
			char, _, err := bufReader.ReadRune()
			if err != nil {
				if err != io.EOF {
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
	err = cmd.Wait()
	resultCode := 0
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
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

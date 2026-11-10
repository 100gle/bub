// Command agent is a fake ACP agent the acp package's tests spawn as a real
// process. It implements the baseline agent methods with the SDK's agent side,
// streams one message on every prompt, and exits when its standard input
// closes, as a well-behaved agent does.
//
// It is built by internal/acp's tests, not shipped with the app.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	sdk "github.com/caelis-labs/acp-go-sdk"
	"github.com/caelis-labs/acp-go-sdk/transport/stdio"
)

type agent struct{}

func (agent) Initialize(_ context.Context, params sdk.InitializeRequest) (sdk.InitializeResponse, error) {
	if params.ProtocolVersion != sdk.ProtocolVersion(sdk.WireProtocolVersion) {
		return sdk.InitializeResponse{}, sdk.NewInvalidParams("unsupported protocol version")
	}
	return sdk.InitializeResponse{
		ProtocolVersion: sdk.ProtocolVersion(sdk.WireProtocolVersion),
		AgentInfo:       &sdk.Implementation{Name: "testdata-agent", Version: "0.0.1"},
	}, nil
}

func (agent) NewSession(_ context.Context, _ sdk.NewSessionRequest) (sdk.NewSessionResponse, error) {
	return sdk.NewSessionResponse{SessionId: "test-session"}, nil
}

func (agent) Prompt(ctx context.Context, params sdk.PromptRequest) (sdk.PromptResponse, error) {
	conn, ok := sdk.AgentSideConnectionFromContext(ctx)
	if !ok {
		return sdk.PromptResponse{}, sdk.NewInternalError("no agent connection in the context")
	}
	update := sdk.UpdateAgentMessageText("hello from the testdata agent")
	if err := conn.SessionUpdate(ctx, sdk.SessionNotification{SessionId: params.SessionId, Update: update}); err != nil {
		return sdk.PromptResponse{}, err
	}
	return sdk.PromptResponse{StopReason: sdk.StopReasonEndTurn}, nil
}

func (agent) Cancel(context.Context, sdk.CancelNotification) error { return nil }

func main() {
	fmt.Fprintln(os.Stderr, "testdata agent ready")
	err := stdio.ServeAgent(context.Background(), agent{}, sdk.ConnectionOptions{})
	if err != nil && !errors.Is(err, sdk.ErrConnectionClosed) && !errors.Is(err, sdk.ErrPeerClosed) {
		fmt.Fprintln(os.Stderr, "testdata agent:", err)
		os.Exit(1)
	}
}

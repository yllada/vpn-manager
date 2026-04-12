// Package tailscale implements privileged handlers for Tailscale CLI operations.
package tailscale

import (
	"github.com/yllada/vpn-manager/daemon"
)

// =============================================================================
// DAEMON HANDLERS
// =============================================================================

// UpHandler returns a handler that runs tailscale up.
func UpHandler(state *daemon.State) daemon.HandlerFunc {
	return func(ctx *daemon.HandlerContext) (any, error) {
		var params UpParams
		if err := ctx.UnmarshalParams(&params); err != nil {
			return nil, err
		}

		ctx.Logger.Printf("Running tailscale up (exit_node: %s, login_server: %s)",
			params.ExitNode, params.LoginServer)

		manager, err := NewManager()
		if err != nil {
			return nil, err
		}

		result, err := manager.Up(ctx.Context, params)
		if err != nil {
			return nil, err
		}

		// Update state
		state.SetTailscale(daemon.TailscaleState{
			Connected:              true,
			ExitNode:               params.ExitNode,
			ExitNodeAllowLANAccess: params.ExitNodeAllowLANAccess,
			LoginServer:            params.LoginServer,
		})

		return result, nil
	}
}

// DownHandler returns a handler that runs tailscale down.
func DownHandler(state *daemon.State) daemon.HandlerFunc {
	return func(ctx *daemon.HandlerContext) (any, error) {
		ctx.Logger.Printf("Running tailscale down")

		manager, err := NewManager()
		if err != nil {
			return nil, err
		}

		if err := manager.Down(ctx.Context); err != nil {
			return nil, err
		}

		// Update state
		state.SetTailscaleConnected(false)

		return map[string]bool{"success": true}, nil
	}
}

// SetHandler returns a handler that runs tailscale set.
func SetHandler(state *daemon.State) daemon.HandlerFunc {
	return func(ctx *daemon.HandlerContext) (any, error) {
		var params SetParams
		if err := ctx.UnmarshalParams(&params); err != nil {
			return nil, err
		}

		ctx.Logger.Printf("Running tailscale set")

		manager, err := NewManager()
		if err != nil {
			return nil, err
		}

		result, err := manager.Set(ctx.Context, params)
		if err != nil {
			return nil, err
		}

		// Update state if exit node changed
		if params.ExitNode != nil {
			ts := state.GetTailscale()
			ts.ExitNode = *params.ExitNode
			state.SetTailscale(ts)
		}
		if params.ExitNodeAllowLANAccess != nil {
			ts := state.GetTailscale()
			ts.ExitNodeAllowLANAccess = *params.ExitNodeAllowLANAccess
			state.SetTailscale(ts)
		}

		return result, nil
	}
}

// LoginHandler returns a handler that runs tailscale login.
func LoginHandler(state *daemon.State) daemon.HandlerFunc {
	return func(ctx *daemon.HandlerContext) (any, error) {
		var params LoginParams
		if err := ctx.UnmarshalParams(&params); err != nil {
			return nil, err
		}

		ctx.Logger.Printf("Running tailscale login (server: %s)", params.LoginServer)

		manager, err := NewManager()
		if err != nil {
			return nil, err
		}

		result, err := manager.Login(ctx.Context, params)
		if err != nil {
			return nil, err
		}

		// Update state
		if result.Success && result.AuthURL == "" {
			// Login completed without needing browser
			state.SetTailscaleConnected(true)
		}

		return result, nil
	}
}

// LogoutHandler returns a handler that runs tailscale logout.
func LogoutHandler(state *daemon.State) daemon.HandlerFunc {
	return func(ctx *daemon.HandlerContext) (any, error) {
		ctx.Logger.Printf("Running tailscale logout")

		manager, err := NewManager()
		if err != nil {
			return nil, err
		}

		if err := manager.Logout(ctx.Context); err != nil {
			return nil, err
		}

		// Update state
		state.SetTailscale(daemon.TailscaleState{})

		return map[string]bool{"success": true}, nil
	}
}

// SetOperatorHandler returns a handler that sets the tailscale operator.
func SetOperatorHandler(state *daemon.State) daemon.HandlerFunc {
	return func(ctx *daemon.HandlerContext) (any, error) {
		var params struct {
			Username string `json:"username"`
		}
		if err := ctx.UnmarshalParams(&params); err != nil {
			return nil, err
		}

		ctx.Logger.Printf("Setting tailscale operator to: %s", params.Username)

		manager, err := NewManager()
		if err != nil {
			return nil, err
		}

		if err := manager.SetOperator(ctx.Context, params.Username); err != nil {
			return nil, err
		}

		return map[string]any{
			"success":  true,
			"operator": params.Username,
		}, nil
	}
}

// TaildropSendHandler returns a handler that sends a file via Taildrop.
func TaildropSendHandler(state *daemon.State) daemon.HandlerFunc {
	return func(ctx *daemon.HandlerContext) (any, error) {
		var params TaildropSendParams
		if err := ctx.UnmarshalParams(&params); err != nil {
			return nil, err
		}

		ctx.Logger.Printf("Sending file %s to %s via Taildrop", params.FilePath, params.Target)

		manager, err := NewManager()
		if err != nil {
			return nil, err
		}

		result, err := manager.SendFile(ctx.Context, params)
		if err != nil {
			return nil, err
		}

		return result, nil
	}
}

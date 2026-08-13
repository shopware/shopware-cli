package runtimectx

import (
	"cli/internal/auth"
	"cli/internal/config"
	"context"
	"log/slog"
)

// Package runtimectx provides helper functions for storing and retrieving
// runtime values such as configuration and logger instances in a context.Context.
//
// This design keeps the CLI setup modular and testable: the Root command
// prepares the context with bootstrap steps, and subcommands retrieve the
// values they need from the context instead of relying on global variables.

type (
	// loggerKey is an unexported type used as the key for storing a logger in a Context.
	// Using a distinct, unexported type prevents collisions with context keys defined in other packages,
	// as recommended by the Go documentation.
	loggerKey struct{}

	// configKey is an unexported type used as the key for storing a config in a Context.
	// Using a distinct, unexported type prevents collisions with context keys defined in other packages,
	// as recommended by the Go documentation.
	configKey struct{}

	authKey struct{}
)

// WithConfig returns a new Context that carries the given Config.
// The value can later be retrieved with ConfigFrom().
// This is intended to pass the runtime configuration through the call chain
// without relying on global state.
// Typical usage: ctx = runtimectx.WithConfig(ctx, cfg)
func WithConfig(ctx context.Context, config config.Config) context.Context {
	return context.WithValue(ctx, configKey{}, config)
}

// ConfigFrom extracts the Config value from the given Context.
// It returns the stored config.Config if present and of the correct type,
// or an empty config.Config if no configuration was found.
// Use together with WithConfig to propagate configuration without globals.
// Typical usage: cfg := runtimectx.ConfigFrom(ctx)
func ConfigFrom(ctx context.Context) config.Config {
	configVal := ctx.Value(configKey{})
	if configVal != nil {
		if cfg, ok := configVal.(config.Config); ok {
			return cfg
		}
	}
	return config.Config{}
}

// WithLogger returns a new Context that carries the given logger.
// The value can later be retrieved with LoggerFrom().
// This is intended to pass the runtime configuration through the call chain
// without relying on global state.
// Typical usage: ctx = runtimectx.WithLogger(ctx, logger)
func WithLogger(ctx context.Context, logger slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

// LoggerFrom extracts the logger value from the given Context.
// It returns the stored slog.Logger if present and of the correct type,
// or an empty logger if none was found.
// Use together with WithLogger to propagate configuration without globals.
// Typical usage: lg, _  := runtimectx.LoggerFrom[*slog.Logger](ctx)
func LoggerFrom(ctx context.Context) slog.Logger {
	loggerVal := ctx.Value(loggerKey{})
	if loggerVal != nil {
		logger, ok := loggerVal.(slog.Logger)
		if ok {
			return logger
		}
	}
	return slog.Logger{}
}

// WithAuthClient returns a new Context that carries the given authentication client.
// The value can later be retrieved with AuthClientFrom().
// This is intended to pass the authentication client through the call chain
// without relying on global state.
// Typical usage: ctx = runtimectx.WithAuthClient(ctx, authClient)
func WithAuthClient(ctx context.Context, authClient auth.Client) context.Context {
	return context.WithValue(ctx, authKey{}, authClient)
}

// AuthClientFrom extracts the authentication client from the given Context.
// It returns the stored auth.Client if present and of the correct type,
// or an empty auth.Client if none was found.
// Use together with WithAuthClient to propagate authentication state without globals.
// Typical usage: client := runtimectx.AuthClientFrom(ctx)
func AuthClientFrom(ctx context.Context) auth.Client {
	authVal := ctx.Value(authKey{})
	if authVal != nil {
		authClient, ok := authVal.(auth.Client)
		if ok {
			return authClient
		}
	}
	return auth.Client{}
}

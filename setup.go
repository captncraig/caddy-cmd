package cmd

import (
	"github.com/mholt/caddy/caddy/setup"
	"github.com/mholt/caddy/middleware"
	"time"
)

func Setup(c *setup.Controller) (middleware.Middleware, error) {
	module, err := parse(c)
	if err != nil {
		return nil, err
	}
	module.root = c.Root
	return func(next middleware.Handler) middleware.Handler {
		module.next = next
		return module
	}, nil
}

func parse(c *setup.Controller) (*cmdModule, error) {
	module := &cmdModule{}
	for c.Next() {
		args := c.RemainingArgs()
		if len(args) == 0 {
			return nil, c.ArgErr()
		}
		cmd := &command{
			Path:            args[0],
			Timeout:         time.Minute,
			lock:            make(chan bool, 1),
			Method:          "POST",
			AllowConcurrent: false,
		}
		module.Commands = append(module.Commands, cmd)
		if len(args) > 1 {
			cmd.Execs = []*ex{
				&ex{
					Command: args[1],
					Args:    args[2:],
				},
			}
		}
		for c.NextBlock() {
			switch c.Val() {
			case "exec":
				args := c.RemainingArgs()
				if len(args) == 0 {
					return nil, c.ArgErr()
				}
				exe := &ex{
					Command: args[0],
					Args:    args[1:],
				}
				cmd.Execs = append(cmd.Execs, exe)
			case "timeout":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				t, err := time.ParseDuration(args[0])
				if err != nil {
					return nil, err
				}
				cmd.Timeout = t
			case "method":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				cmd.Method = args[0]
			case "description":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				cmd.Description = args[0]
			case "ui":
				args := c.RemainingArgs()
				if len(args) != 1 {
					return nil, c.ArgErr()
				}
				if module.uiPath != "" {
					return nil, c.Err("Can only set ui once")
				}
				module.uiPath = args[0]
			case "multiple":
				args := c.RemainingArgs()
				if len(args) != 0 {
					return nil, c.ArgErr()
				}
				cmd.AllowConcurrent = true
			case "shell":
				args := c.RemainingArgs()
				if len(args) != 0 {
					return nil, c.ArgErr()
				}
				cmd.shell = true
			default:
				return nil, c.Errf("Invalid cmd args %s", c.Val())
			}
		}
	}
	return module, nil
}

package entconfig

import (
	"fmt"

	"github.com/go-gcfg/gcfg"

	"github.com/launchdarkly/ld-relay/v6/core/config"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
)

// LoadConfigFile reads a configuration file into an EnterpriseConfig struct and performs basic validation.
//
// The Config parameter could be initialized with default values first, but does not need to be.
func LoadConfigFile(c *EnterpriseConfig, path string, loggers ldlog.Loggers) error {
	if err := gcfg.ReadFileInto(c, path); err != nil {
		return fmt.Errorf(`failed to read configuration file "%s": %w`, path, config.FilterGcfgError(err))
	}

	return ValidateConfig(c, loggers)
}

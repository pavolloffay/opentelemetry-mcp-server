/*
 * Copyright 2023 SentinelOne, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package config

import (
	"fmt"

	"github.com/scalyr/dataset-go/pkg/server_host_config"

	osUtil "github.com/scalyr/dataset-go/internal/pkg/os/util"
	"github.com/scalyr/dataset-go/pkg/buffer_config"
)

const DebugModeDefault = false

// DataSetTokens wrap DataSet access tokens
type DataSetTokens struct {
	WriteLog    string
	ReadLog     string
	WriteConfig string
	ReadConfig  string
}

func (tokens DataSetTokens) String() string {
	return fmt.Sprintf(
		"WriteLog: %t, ReadLog: %t, WriteConfig: %t, ReadConfig: %t",
		tokens.WriteLog != "",
		tokens.ReadLog != "",
		tokens.WriteConfig != "",
		tokens.ReadConfig != "",
	)
}

// DataSetConfig wraps DataSet endpoint configuration (host, tokens, etc.)
type DataSetConfig struct {
	Endpoint           string
	Tokens             DataSetTokens
	BufferSettings     buffer_config.DataSetBufferSettings
	ServerHostSettings server_host_config.DataSetServerHostSettings
	Debug              bool
}

func NewDefaultDataSetConfig() DataSetConfig {
	return DataSetConfig{
		Endpoint:           "https://app.scalyr.com",
		Tokens:             DataSetTokens{},
		BufferSettings:     buffer_config.NewDefaultDataSetBufferSettings(),
		ServerHostSettings: server_host_config.NewDefaultDataSetServerHostSettings(),
		Debug:              DebugModeDefault,
	}
}

type DataSetConfigOption func(*DataSetConfig) error

func WithEndpoint(endpoint string) DataSetConfigOption {
	return func(c *DataSetConfig) error {
		c.Endpoint = endpoint
		return nil
	}
}

func WithTokens(tokens DataSetTokens) DataSetConfigOption {
	return func(c *DataSetConfig) error {
		c.Tokens = tokens
		return nil
	}
}

func WithBufferSettings(bufferSettings buffer_config.DataSetBufferSettings) DataSetConfigOption {
	return func(c *DataSetConfig) error {
		c.BufferSettings = bufferSettings
		return nil
	}
}

func WithServerHostSettings(serverHostSettings server_host_config.DataSetServerHostSettings) DataSetConfigOption {
	return func(c *DataSetConfig) error {
		c.ServerHostSettings = serverHostSettings
		return nil
	}
}

func WithDebug(debug bool) DataSetConfigOption {
	return func(c *DataSetConfig) error {
		c.Debug = debug
		return nil
	}
}

func FromEnv() DataSetConfigOption {
	return func(c *DataSetConfig) error {
		if c.Tokens.WriteLog == "" {
			c.Tokens.WriteLog = osUtil.GetEnvVariableOrDefault("SCALYR_WRITELOG_TOKEN", "")
		}
		if c.Tokens.ReadLog == "" {
			c.Tokens.ReadLog = osUtil.GetEnvVariableOrDefault("SCALYR_READLOG_TOKEN", "")
		}
		if c.Tokens.ReadConfig == "" {
			c.Tokens.ReadConfig = osUtil.GetEnvVariableOrDefault("SCALYR_READCONFIG_TOKEN", "")
		}
		if c.Tokens.WriteConfig == "" {
			c.Tokens.WriteConfig = osUtil.GetEnvVariableOrDefault("SCALYR_WRITECONFIG_TOKEN", "")
		}
		if c.Endpoint == "" {
			c.Endpoint = osUtil.GetEnvVariableOrDefault("SCALYR_SERVER", "")
		}
		c.BufferSettings = buffer_config.NewDefaultDataSetBufferSettings()
		c.ServerHostSettings = server_host_config.NewDefaultDataSetServerHostSettings()
		c.Debug = DebugModeDefault
		return nil
	}
}

func New(opts ...DataSetConfigOption) (*DataSetConfig, error) {
	cfg := &DataSetConfig{}
	for _, opt := range opts {
		if err := opt(cfg); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

func (cfg *DataSetConfig) WithOptions(opts ...DataSetConfigOption) (*DataSetConfig, error) {
	newCfg := *cfg
	for _, opt := range opts {
		if err := opt(&newCfg); err != nil {
			return &newCfg, err
		}
	}
	return &newCfg, nil
}

func (cfg *DataSetConfig) String() string {
	return fmt.Sprintf(
		"Endpoint: %s, Tokens: (%s), BufferSettings: (%s), ServerHostSettings: (%s), Debug: (%t)",
		cfg.Endpoint,
		cfg.Tokens.String(),
		cfg.BufferSettings.String(),
		cfg.ServerHostSettings.String(),
		cfg.Debug,
	)
}

func (cfg *DataSetConfig) Validate() error {
	if cfg.Endpoint == "" {
		return fmt.Errorf("endpoint cannot be empty")
	}
	bufferErr := cfg.BufferSettings.Validate()
	if bufferErr != nil {
		return fmt.Errorf("buffer settings are invalid: %w", bufferErr)
	}
	serverHostErr := cfg.ServerHostSettings.Validate()
	if bufferErr != nil {
		return fmt.Errorf("server host settings are invalid: %w", serverHostErr)
	}
	return nil
}

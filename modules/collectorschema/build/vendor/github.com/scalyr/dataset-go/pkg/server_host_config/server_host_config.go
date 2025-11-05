package server_host_config

import "fmt"

type DataSetServerHostSettings struct {
	UseHostName bool
	ServerHost  string
}

func NewDefaultDataSetServerHostSettings() DataSetServerHostSettings {
	return DataSetServerHostSettings{
		UseHostName: true,
		ServerHost:  "",
	}
}

type DataSetServerHostSettingsOption func(*DataSetServerHostSettings) error

func WithUseHostName(useHostName bool) DataSetServerHostSettingsOption {
	return func(c *DataSetServerHostSettings) error {
		c.UseHostName = useHostName
		return nil
	}
}

func WithServerHost(serverHost string) DataSetServerHostSettingsOption {
	return func(c *DataSetServerHostSettings) error {
		c.ServerHost = serverHost
		return nil
	}
}

func New(opts ...DataSetServerHostSettingsOption) (*DataSetServerHostSettings, error) {
	cfg := &DataSetServerHostSettings{}
	for _, opt := range opts {
		if err := opt(cfg); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

func (cfg *DataSetServerHostSettings) WithOptions(opts ...DataSetServerHostSettingsOption) (*DataSetServerHostSettings, error) {
	newCfg := *cfg
	for _, opt := range opts {
		if err := opt(&newCfg); err != nil {
			return &newCfg, err
		}
	}
	return &newCfg, nil
}

func (cfg *DataSetServerHostSettings) String() string {
	return fmt.Sprintf(
		"UseHostName: %t, ServerHost: %s",
		cfg.UseHostName,
		cfg.ServerHost,
	)
}

func (cfg *DataSetServerHostSettings) Validate() error {
	if !cfg.UseHostName && len(cfg.ServerHost) == 0 {
		return fmt.Errorf("when UseHostName is False, then ServerHost has to be set")
	}
	return nil
}

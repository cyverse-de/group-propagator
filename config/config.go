package config

import (
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

type Config struct {
	IplantGroupsBase             string
	IplantGroupsUser             string
	IplantGroupsFolderNamePrefix string
	IplantGroupsPublicGroup      string

	DataInfoBase string
	IRODSUser    string

	AMQPURI          string
	AMQPExchangeName string
	AMQPExchangeType string
	AMQPQueuePrefix  string
}

func NewFromViper(cfg *viper.Viper) (*Config, error) {
	c := &Config{
		IplantGroupsBase:             cfg.GetString("iplant_groups.base"),
		IplantGroupsUser:             cfg.GetString("iplant_groups.user"),
		IplantGroupsFolderNamePrefix: cfg.GetString("iplant_groups.folder_name_prefix"),
		IplantGroupsPublicGroup:      cfg.GetString("iplant_groups.public_group"),

		DataInfoBase: cfg.GetString("data_info.base"),
		IRODSUser:    cfg.GetString("irods.user"),

		AMQPURI:          cfg.GetString("amqp.uri"),
		AMQPExchangeName: cfg.GetString("amqp.exchange.name"),
		AMQPExchangeType: cfg.GetString("amqp.exchange.type"),
		AMQPQueuePrefix:  cfg.GetString("amqp.queue_prefix"),
	}

	err := c.Validate()
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Config) Validate() error {
	var errorkeys []string

	if c.IplantGroupsBase == "" {
		errorkeys = append(errorkeys, "iplant_groups.base")
	}
	if c.IplantGroupsUser == "" {
		errorkeys = append(errorkeys, "iplant_groups.user")
	}
	if c.IplantGroupsFolderNamePrefix == "" {
		errorkeys = append(errorkeys, "iplant_groups.folder_name_prefix")
	}
	if c.IplantGroupsPublicGroup == "" {
		errorkeys = append(errorkeys, "iplant_groups.public_group")
	}

	if c.DataInfoBase == "" {
		errorkeys = append(errorkeys, "data_info.base")
	}
	if c.IRODSUser == "" {
		errorkeys = append(errorkeys, "irods.user")
	}

	if c.AMQPURI == "" {
		errorkeys = append(errorkeys, "amqp.uri")
	}
	if c.AMQPExchangeName == "" {
		errorkeys = append(errorkeys, "amqp.exchange.name")
	}
	if c.AMQPExchangeType == "" {
		errorkeys = append(errorkeys, "amqp.exchange.type")
	}
	// AMQPQueuePrefix can be the empty string (usually will be, probably)

	if len(errorkeys) > 0 {
		return errors.Errorf("Configuration keys must be set: %s", strings.Join(errorkeys, ", "))
	}
	return nil
}

package config

import (
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

type Config struct {
	GroupsBase       string
	GroupsUser       string
	DEUsersGroupName string

	DataInfoBase string
	IRODSUser    string

	AMQPURI          string
	AMQPExchangeName string
	AMQPExchangeType string
	AMQPQueuePrefix  string
}

func NewFromViper(cfg *viper.Viper) (*Config, error) {
	c := &Config{
		GroupsBase:       cfg.GetString("groups.base"),
		GroupsUser:       cfg.GetString("groups.user"),
		DEUsersGroupName: cfg.GetString("groups.de_users_group"),

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

	if c.GroupsBase == "" {
		errorkeys = append(errorkeys, "groups.base")
	}
	if c.GroupsUser == "" {
		errorkeys = append(errorkeys, "groups.user")
	}
	if c.DEUsersGroupName == "" {
		errorkeys = append(errorkeys, "groups.de_users_group")
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

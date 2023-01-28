package main

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/cyverse-de/configurate"
	l "github.com/cyverse-de/go-mod/logging"
	"github.com/cyverse-de/go-mod/otelutils"
	"github.com/cyverse-de/messaging/v9"

	"github.com/cyverse-de/group-propagator/client/datainfo"
	"github.com/cyverse-de/group-propagator/client/groups"
	"github.com/cyverse-de/group-propagator/config"
	"github.com/cyverse-de/group-propagator/logging"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/streadway/amqp"
)

var log = logging.Log.WithFields(logrus.Fields{"package": "main"})

const serviceName = "group-propagator"

const otelName = "github.com/cyverse-de/group-propagator"

const defaultConfig = `
amqp:
  uri: amqp://guest:guest@rabbit:5672/
  queue_prefix: ""
  exchange:
    name: de
    type: topic

iplant_groups:
  base: "http://iplant-groups"
  user: GrouperSystem
  folder_name_prefix: "iplant:de:notprod"
  public_group: 7777777777777777777777777777777777

data_info:
  base: "http://data-info"

irods:
  user: "de-irods"
`

func getQueueName(prefix string) string {
	if len(prefix) > 0 {
		return fmt.Sprintf("%s.%s", prefix, serviceName)
	}
	return serviceName
}

// A spinner to keep the program running since client.Listen() needs to be in a goroutine.
//nolint
func spin() {
	spinner := make(chan int)
	for {
		select {
		case <-spinner:
			fmt.Println("Exiting")
			break
		}
	}
}

func main() {
	var (
		cfgPath  = flag.String("config", "/etc/iplant/de/group-propagator.yml", "The path to the config file")
		logLevel = flag.String("log-level", "info", "One of trace, debug, info, warn, error, fatal, or panic.")

		err error
		cfg *viper.Viper
	)

	flag.Parse()
	l.SetupLogging(*logLevel)

	var tracerCtx, cancel = context.WithCancel(context.Background())
	defer cancel()
	shutdown := otelutils.TracerProviderFromEnv(tracerCtx, serviceName, func(e error) { log.Fatal(e) })
	defer shutdown()

	if *cfgPath == "" {
		log.Fatal("--config must not be the empty string")
	}

	if cfg, err = configurate.InitDefaults(*cfgPath, defaultConfig); err != nil {
		log.Fatal(err.Error())
	}

	// package up config nicely
	configuration, err := config.NewFromViper(cfg)
	if err != nil {
		log.Fatal(errors.Wrap(err, "Couldn't validate configuration"))
	}

	// Set up AMQP
	listenClient, err := messaging.NewClient(configuration.AMQPURI, true)
	if err != nil {
		log.Fatal(errors.Wrap(err, "Unable to create the messaging listen client"))
	}
	defer listenClient.Close()

	publishClient, err := messaging.NewClient(configuration.AMQPURI, true)
	if err != nil {
		log.Fatal(errors.Wrap(err, "Unable to create the messaging publish client"))
	}
	defer publishClient.Close()

	err = publishClient.SetupPublishing(configuration.AMQPExchangeName)
	if err != nil {
		log.Fatal(errors.Wrap(err, "Unable to set up message publishing"))
	}

	go listenClient.Listen()

	// Create clients
	gc := groups.NewGroupsClient(configuration.IplantGroupsBase, configuration.IplantGroupsUser)
	err = gc.Check(context.Background())
	if err != nil {
		log.Fatal(errors.Wrap(err, "Couldn't ping iplant-groups"))
	} else {
		log.Info("Pinged iplant-groups successfully")
	}

	dc := datainfo.NewDataInfoClient(configuration.DataInfoBase, configuration.IRODSUser)
	err = dc.Check(context.Background())
	if err != nil {
		log.Fatal(errors.Wrap(err, "Couldn't ping data-info"))
	} else {
		log.Info("Pinged data-info successfully")
	}

	propagator := NewPropagator(gc, "@grouper-", dc)
	crawler := NewCrawler(gc, configuration.IplantGroupsFolderNamePrefix, configuration.IplantGroupsPublicGroup, publishClient)

	queueName := getQueueName(configuration.AMQPQueuePrefix)
	listenClient.AddConsumerMulti(
		configuration.AMQPExchangeName,
		configuration.AMQPExchangeType,
		queueName,
		[]string{"index.all", "index.groups", "index.group.#"},
		func(ctx context.Context, del amqp.Delivery) {
			var err error
			log.Tracef("Got message: %s", del.RoutingKey)
			if del.RoutingKey == "index.all" || del.RoutingKey == "index.groups" {
				err = crawler.CrawlGrouperGroups(ctx)
			} else if strings.HasPrefix(del.RoutingKey, "index.group.") {
				groupID := del.RoutingKey[len("index.group."):]
				err = propagator.PropagateGroupById(ctx, groupID)
			}

			if err != nil {
				log.Error(errors.Wrap(err, "Error handling message"))
				err = del.Reject(!del.Redelivered)
			} else {
				err = del.Ack(false)
			}

			if err != nil {
				log.Error(errors.Wrap(err, fmt.Sprintf("Error ack/rejecting message: %s", del.RoutingKey)))
			}
		},
		1)

	spin() // unless we want to add an HTTP API of some sort, maybe
}

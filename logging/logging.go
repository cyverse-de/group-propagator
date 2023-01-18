package logging

import (
	"github.com/cyverse-de/go-mod/logging"
	"github.com/sirupsen/logrus"
)

var Log = logging.Log.WithFields(logrus.Fields{
	"service": "group-propagator",
})

// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package nrlogrus sends go-agent log messages to
// https://github.com/sirupsen/logrus.
//
// Use this package if you are using logrus in your application and would like
// the go-agent log messages to end up in the same place.  If you are using
// the logrus standard logger, use ConfigStandardLogger when creating your
// application:
//
//	app, err := newrelic.NewApplication(
//		newrelic.ConfigFromEnvironment(),
//		nrlogrus.ConfigStandardLogger(),
//	)
//
// If you are using a particular logrus Logger instance, then use ConfigLogger:
//
//	l := logrus.New()
//	l.SetLevel(logrus.DebugLevel)
//	app, err := newrelic.NewApplication(
//		newrelic.ConfigFromEnvironment(),
//		nrlogrus.ConfigLogger(l),
//	)
//
// This package requires logrus version v1.1.0 and above.
package nrlogrus

import (
	"github.com/newrelic/go-agent/v3/internal"
	newrelic "github.com/newrelic/go-agent/v3/newrelic"
	"github.com/sirupsen/logrus"
)

func init() { internal.TrackUsage("integration", "logging", "logrus") }

type shim struct {
	e *logrus.Entry
	l *logrus.Logger
}

func (s *shim) Error(msg string, c map[string]interface{}) {
	s.e.WithFields(c).Error(msg)
}
func (s *shim) Warn(msg string, c map[string]interface{}) {
	s.e.WithFields(c).Warn(msg)
}
func (s *shim) Info(msg string, c map[string]interface{}) {
	s.e.WithFields(c).Info(msg)
}
func (s *shim) Debug(msg string, c map[string]interface{}) {
	s.e.WithFields(c).Debug(msg)
}
func (s *shim) DebugEnabled() bool {
	lvl := s.l.GetLevel()
	return lvl >= logrus.DebugLevel
}

// StandardLogger returns a newrelic.Logger which forwards agent log messages to
// the logrus package-level exported logger.
func StandardLogger() newrelic.Logger {
	return Transform(logrus.StandardLogger())
}

// Transform turns a *logrus.Logger into a newrelic.Logger.
func Transform(l *logrus.Logger) newrelic.Logger {
	return &shim{
		l: l,
		e: l.WithFields(logrus.Fields{
			"component": "newrelic",
		}),
	}
}

// ConfigLogger configures the newrelic.Application to send log messsages to the
// provided logrus logger.
func ConfigLogger(l *logrus.Logger) newrelic.ConfigOption {
	return newrelic.ConfigLogger(Transform(l))
}

// ConfigStandardLogger configures the newrelic.Application to send log
// messsages to the standard logrus logger.
func ConfigStandardLogger() newrelic.ConfigOption {
	return newrelic.ConfigLogger(StandardLogger())
}

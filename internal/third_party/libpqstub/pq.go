package pq

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
)

func init() { sql.Register("postgres", stubDriver{}) }

var errStub = errors.New("github.com/lib/pq stub: real driver unavailable in this environment")

type stubDriver struct{}

func (stubDriver) Open(name string) (driver.Conn, error) { return nil, errStub }
func (stubDriver) OpenConnector(name string) (driver.Connector, error) {
	return stubConnector{name: name}, nil
}

type stubConnector struct{ name string }

func (c stubConnector) Connect(context.Context) (driver.Conn, error) { return nil, errStub }
func (c stubConnector) Driver() driver.Driver                        { return stubDriver{} }

package test

import (
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/pablintino/automation-executor/internal/config"
	"github.com/pablintino/commons-go/command"
	"github.com/pablintino/testing-go/containers"
	"path/filepath"
	"sync"
	"testing"
)

type TBDb interface {
	GetDBConfig() config.DatabaseConfig
	Recreate() error
	release() error
}

type TBDbSettings struct {
	EncryptionKey      string
	PostgreSqlSettings *containers.PostgreSqlContainerRequest
}

type TBDbSettingsBuilder interface {
	SetEncryptionKey(key string) TBDbSettingsBuilder
	Build() *TBDbSettings
}

type TBDbImpl struct {
	paths           TestPaths
	settings        TBDbSettings
	ctrIface        containers.ContainerInterface
	dbContainer     containers.PostgreSqlContainer
	generatedConfig config.DatabaseConfig
	createMutex     sync.Mutex
}

func NewTBDbImpl(paths TestPaths, settings *TBDbSettings, ctrIface containers.ContainerInterface) (*TBDbImpl, error) {
	if settings == nil {
		settings = &TBDbSettings{
			EncryptionKey: uuid.New().String(),
		}
	}
	if settings.PostgreSqlSettings == nil {
		settings.PostgreSqlSettings = containers.NewPosgresSqlRequestBuilder().
			WithInitScript(filepath.Join(paths.DbScripts(), "db-schema.sql")).
			Build()
	}
	inst := &TBDbImpl{paths: paths, settings: *settings, ctrIface: ctrIface}
	if err := inst.createDb(); err != nil {
		return nil, err
	}
	return inst, nil
}

func (tb *TBDbImpl) release() error {
	tb.createMutex.Lock()
	defer tb.createMutex.Unlock()
	if tb.dbContainer != nil {
		if _, err := tb.dbContainer.Destroy(); err != nil {
			return err
		}
	}
	return nil
}

func (tb *TBDbImpl) GetDBConfig() config.DatabaseConfig {
	return tb.generatedConfig
}

func (tb *TBDbImpl) Recreate() error {
	if err := tb.release(); err != nil {
		return err
	}
	return tb.createDb()
}

func (tb *TBDbImpl) createDb() error {
	tb.createMutex.Lock()
	defer tb.createMutex.Unlock()
	var err error

	tb.dbContainer, err = containers.NewPostgreSqlContainer(tb.ctrIface, tb.settings.PostgreSqlSettings)
	if err != nil {
		return err
	}
	tb.generatedConfig = config.DatabaseConfig{
		DataSource:    tb.dbContainer.GetConnectionString(),
		EncryptionKey: tb.settings.EncryptionKey,
	}
	return nil
}

type TB interface {
	DB() TBDb
	Paths() TestPaths
	Release()
}

type TBImpl struct {
	t        *testing.T
	db       TBDb
	paths    TestPaths
	ctrIface containers.ContainerInterface
}

func NewTB(t *testing.T, tbDbSettings *TBDbSettings) (*TBImpl, error) {
	tb := &TBImpl{t: t, paths: NewTestPaths(t), ctrIface: containers.NewContainerInterface(command.NewExecCmdFactory())}
	db, err := NewTBDbImpl(tb.paths, tbDbSettings, tb.ctrIface)
	if err != nil {
		return nil, err
	}
	tb.db = db
	return tb, nil
}

func NewTBDefault(t *testing.T) (*TBImpl, error) {
	return NewTB(t, nil)
}

func (tb *TBImpl) Paths() TestPaths {
	return tb.paths
}
func (tb *TBImpl) DB() TBDb {
	return tb.db
}

func (tb *TBImpl) Release() {
	var err error
	if tb.db != nil {
		if tmpErr := tb.db.release(); tmpErr != nil {
			err = fmt.Errorf("failed to clean the TB DB %w", err)
		}
	}
	if tmpErr := tb.ctrIface.Release(); tmpErr != nil {
		err = errors.Join(err, fmt.Errorf("failed to release the TB container interface %w", err))
	}
	if err != nil {
		tb.t.Fatal(err)
	}
}

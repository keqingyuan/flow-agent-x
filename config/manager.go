package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github/flowci/flow-agent-x/domain"
	"github/flowci/flow-agent-x/util"

	"github.com/streadway/amqp"
)

const (
	errSettingConnectFail = "Cannot get settings from server"
)

var (
	singleton *Manager
	once      sync.Once
)

type (
	QueueConfig struct {
		Conn       *amqp.Connection
		Channel    *amqp.Channel
		LogChannel *amqp.Channel
		JobQueue   *amqp.Queue
	}

	// Manager to handle server connection and config
	Manager struct {
		Settings *domain.Settings
		Queue    *QueueConfig
		Zk       *util.ZkClient

		Server string
		Token  string
		Port   int

		// app vars settings
		Vars *domain.Variables

		IsOffline  bool
		Workspace  string
		LoggingDir string
		PluginDir  string

		Quit chan bool
	}
)

// GetInstance get singleton of config manager
func GetInstance() *Manager {
	once.Do(func() {
		singleton = new(Manager)
		singleton.IsOffline = false
		singleton.Quit = make(chan bool)
	})
	return singleton
}

func (m *Manager) Init() {
	// init dir
	_ = os.MkdirAll(m.Workspace, os.ModePerm)
	_ = os.MkdirAll(m.LoggingDir, os.ModePerm)
	_ = os.MkdirAll(m.PluginDir, os.ModePerm)

	m.Vars = &domain.Variables{
		domain.VarServerUrl: m.Server,
		domain.VarAgentToken: m.Token,
		domain.VarAgentPort: strconv.Itoa(m.Port),
		domain.VarAgentWorkspace: m.Workspace,
		domain.VarAgentPluginDir: m.PluginDir,
		domain.VarAgentLogDir: m.LoggingDir,
	}

	// load config and init rabbitmq, zookeeper
	err := func() error {
		var err = loadSettings(m)
		if util.HasError(err) {
			return err
		}

		err = initRabbitMQ(m)
		if util.HasError(err) {
			return err
		}

		return initZookeeper(m)
	}()

	if util.LogIfError(err) {
		toOfflineMode(m)
		return
	}
}

// HasQueue has rabbit mq connected
func (m *Manager) HasQueue() bool {
	return m.Queue != nil
}

// HasZookeeper has zookeeper connected
func (m *Manager) HasZookeeper() bool {
	return m.Zk != nil
}

// Close release resources and connections
func (m *Manager) Close() {
	if m.HasQueue() {
		_ = m.Queue.Channel.Close()
		_ = m.Queue.LogChannel.Close()
		_ = m.Queue.Conn.Close()
	}

	if m.HasZookeeper() {
		m.Zk.Close()
	}
}

func toOfflineMode(m *Manager) {
	util.LogInfo("Mode: 'offline'")
	m.IsOffline = true
}

func loadSettings(m *Manager) error {
	uri := m.Server + "/agents/connect"
	body, _ := json.Marshal(domain.AgentInit{
		Port: m.Port,
		Os:   util.OS(),
	})

	request, _ := http.NewRequest("POST", uri, bytes.NewBuffer(body))
	request.Header.Set(util.HttpHeaderContentType, util.HttpMimeJson)
	request.Header.Set(util.HttpHeaderAgentToken, m.Token)

	resp, errFromReq := http.DefaultClient.Do(request)
	if errFromReq != nil {
		return fmt.Errorf("%s: %v", errSettingConnectFail, errFromReq)
	}

	defer resp.Body.Close()
	raw, _ := ioutil.ReadAll(resp.Body)

	var message domain.SettingsResponse
	errFromJSON := json.Unmarshal(raw, &message)

	if errFromJSON != nil {
		return errFromJSON
	}

	if !message.IsOk() {
		return fmt.Errorf(message.Message)
	}

	m.Settings = message.Data
	util.LogDebug("Settings been loaded from server: \n%v", m.Settings)
	return nil
}

func initRabbitMQ(m *Manager) error {
	if m.Settings == nil {
		return ErrSettingsNotBeenLoaded
	}

	// get connection
	connStr := m.Settings.Queue.GetConnectionString()
	conn, err := amqp.Dial(connStr)
	if err != nil {
		return err
	}

	// create channel for job queue and send back the result
	ch, err := conn.Channel()
	if err != nil {
		return err
	}

	// create channel for push log to server
	logCh, err := conn.Channel()
	if err != nil {
		return err
	}

	// init queue config
	qc := new(QueueConfig)
	qc.Conn = conn
	qc.Channel = ch
	qc.LogChannel = logCh

	// init queue to receive job
	jobQueue, err := ch.QueueDeclare(m.Settings.Agent.GetQueueName(), false, false, false, false, nil)
	qc.JobQueue = &jobQueue

	m.Queue = qc
	return nil
}

func initZookeeper(m *Manager) error {
	if m.Settings == nil {
		return ErrSettingsNotBeenLoaded
	}

	zkConfig := m.Settings.Zookeeper

	// make connection of zk
	client := new(util.ZkClient)
	err := client.Connect(zkConfig.Host)

	if err != nil {
		return err
	}

	m.Zk = client

	// register agent on zk
	agentPath := getZkPath(m.Settings)
	_, nodeErr := client.Create(agentPath, util.ZkNodeTypeEphemeral, string(domain.AgentIdle))

	if nodeErr == nil {
		util.LogInfo("The zk node '%s' has been registered", agentPath)
		return nil
	}

	return nodeErr
}

func getZkPath(s *domain.Settings) string {
	return s.Zookeeper.Root + "/" + s.Agent.ID
}

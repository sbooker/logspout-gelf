package gelf

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Graylog2/go-gelf/gelf"
	"github.com/gliderlabs/logspout/router"
)

var hostname string

func init() {
	hostname, _ = os.Hostname()
	router.AdapterFactories.Register(NewGelfAdapter, "gelf")
}

// GelfAdapter is an adapter that streams UDP JSON to Graylog
type GelfAdapter struct {
	writer *gelf.Writer
	route  *router.Route
}

// NewGelfAdapter creates a GelfAdapter with UDP as the default transport.
func NewGelfAdapter(route *router.Route) (router.LogAdapter, error) {
	_, found := router.AdapterTransports.Lookup(route.AdapterTransport("udp"))
	if !found {
		return nil, errors.New("unable to find adapter: " + route.Adapter)
	}

	gelfWriter, err := gelf.NewWriter(route.Address)
	if err != nil {
		return nil, err
	}

	gelfWriter.CompressionType = getCompressType()
	gelfWriter.CompressionLevel = getCompressLevel()

	return &GelfAdapter{
		route:  route,
		writer: gelfWriter,
	}, nil
}

func getCompressLevel() int {
	if value, isSet := os.LookupEnv("COMPRESS_LEVEL"); isSet {
		level, err := strconv.Atoi(value)
		if err != nil && level >= -1 && level <= 9 {
			return level
		}
	}

	return -1
}

func getCompressType() gelf.CompressType {
	switch strings.ToLower(os.Getenv("COMPRESS_TYPE")) {
	case "none":
		return gelf.CompressNone
	case "zlib":
		return gelf.CompressZlib
	case "gzip":
		return gelf.CompressGzip
	}

	return gelf.CompressGzip
}

// Stream implements the router.LogAdapter interface.
func (a *GelfAdapter) Stream(logstream chan *router.Message) {
	for message := range logstream {

		m := &GelfMessage{message}

		extra, err := m.getExtraFields()
		if err != nil {
			log.Println("Graylog:", err)
			continue
		}

		msg := gelf.Message{
			Version:  "1.1",
			Host:     hostname,
			Short:    m.getShortMessage(),
			Level:    m.getLevel(),
			Facility: m.getFacility(),
			RawExtra: extra,
		}

		if value, isSet := os.LookupEnv("SEND_TIMESTAMP"); isSet && value == "1" {
			msg.TimeUnix = m.getTimestamp()
		}

		if err := a.writer.WriteMessage(&msg); err != nil {
			log.Println("Graylog:", err)
			continue
		}
	}
}

type GelfMessage struct {
	*router.Message
}

func (m GelfMessage) getTimestamp() float64 {
	return float64(m.Message.Time.UnixNano() / int64(time.Millisecond))
}

func (m GelfMessage) getFacility() string {
	return m.getParsedAppMessagePart(5)
}

func (m GelfMessage) getContext() map[string]interface{} {
	return m.getJsonPart(8)
}

func (m GelfMessage) getExtra() map[string]interface{} {
	return m.getJsonPart(9)
}

func (m GelfMessage) getJsonPart(part int8) map[string]interface{} {
	var dat map[string]interface{}
	err := json.Unmarshal([]byte(m.getParsedAppMessagePart(part)), &dat)
	if err != nil {
		return make(map[string]interface{}, 0)
	}

	return dat
}

func (m GelfMessage) getLevel() int32 {
	appLevel := m.getParsedAppMessagePart(6)

	levelMap := map[string]int32{
		"DEBUG":     gelf.LOG_DEBUG,
		"INFO":      gelf.LOG_INFO,
		"NOTICE":    gelf.LOG_NOTICE,
		"WARNING":   gelf.LOG_WARNING,
		"ERROR":     gelf.LOG_ERR,
		"CRITICAL":  gelf.LOG_CRIT,
		"ALERT":     gelf.LOG_ALERT,
		"EMERGENCY": gelf.LOG_EMERG,
	}

	if level, found := levelMap[appLevel]; found {
		return level
	}

	level := gelf.LOG_INFO
	if m.Source == "stderr" {
		level = gelf.LOG_ERR
	}

	return level
}

func (m GelfMessage) getShortMessage() string {
	message := m.getParsedAppMessagePart(7)
	if message == "" {
		message = m.Message.Data
	}

	return message
}

func (m GelfMessage) getParsedAppMessagePart(part int8) string {
	const timeExp = `\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d{6})?(\+|\-)(\d{4}|\d{2}:\d{2})`
	const facilityExp = `[\w-_]+`
	const levelExp = `[\w]+`
	const messageExp = `.*`
	const jsonExpr = `\{\S+\}|\[\]`

	const TIME = 1
	const FACILITY = 5
	const LEVEL = 6
	const MESSAGE = 7
	const CONTEXT = 8
	const EXTRA = 9

	if part != TIME && part != FACILITY && part != LEVEL && part != MESSAGE && part != CONTEXT && part != EXTRA {
		return ""
	}

	expr := regexp.MustCompile(fmt.Sprintf(`^\[(%s)\]\s(%s)\.(%s):\s(%s)\s(%s)\s(%s)$`, timeExp, facilityExp, levelExp, messageExp, jsonExpr, jsonExpr))
	matches := expr.FindStringSubmatch(m.Message.Data)

	if len(matches) != 10 {
		return ""
	}

	return matches[part]
}

func (m GelfMessage) getExtraFields() (json.RawMessage, error) {

	extra := m.getContainerExtra()

	for key, value := range m.getContext() {
		extra["_"+key] = value
	}

	for key, value := range m.getExtra() {
		extra["_"+key] = value
	}

	for key, value := range getEnvExtra() {
		extra["_"+key] = value
	}

	for name, label := range m.Container.Config.Labels {
		if len(name) > 5 && strings.ToLower(name[0:5]) == "gelf_" {
			extra[name[4:]] = label
		}
	}

	rawExtra, err := json.Marshal(extra)
	if err != nil {
		return nil, err
	}
	return rawExtra, nil
}

func (m GelfMessage) getContainerExtra() map[string]interface{} {
	extra := map[string]interface{}{
		"_container_id":   m.Container.ID,
		"_container_name": m.Container.Name[1:], // might be better to use strings.TrimLeft() to remove the first /
		"_image_id":       m.Container.Image,
		"_image_name":     m.Container.Config.Image,
		"_command":        strings.Join(m.Container.Config.Cmd[:], " "),
		"_created":        m.Container.Created,
	}
	swarmnode := m.Container.Node
	if swarmnode != nil {
		extra["_swarm_node"] = swarmnode.Name
	}

	return extra
}

func getEnvExtra() map[string]interface{} {
	var dat map[string]interface{}

	if value, isSet := os.LookupEnv("EXTRA_JSON"); isSet {

		err := json.Unmarshal([]byte(value), &dat)
		if err != nil {
			return make(map[string]interface{}, 0)
		}

		return dat
	}

	return make(map[string]interface{}, 0)
}

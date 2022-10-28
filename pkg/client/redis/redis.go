package redis

import (
	"context"
	"fmt"
	"github.com/go-redis/redis/v9"
	"github.com/pkg/errors"
	"strconv"
)

var _ Clienter = (*Client)(nil)

type Clienter interface {
	GetReplication(ip string, port int32, password string) (string, error)
	GetSentinelMonitor(sentinelIP string, password string) (string, string, error)
	SetAsMaster(ip string, port int32, password string) error
	SetAsSlave(slaveIP, masterIP string, port int32, password string) error
	SetSentinelMonitor(sentinelIP string, password string, monitor map[string]interface{}) error
}

type Client struct{}

func NewClient() *Client {
	return &Client{}
}

// Get info replication
func (c *Client) GetReplication(ip string, port int32, password string) (string, error) {
	rclient := c.initClient(ip, port, password)

	info, err := rclient.Info(context.Background(), "replication").Result()
	if err != nil {
		return "", errors.Wrap(err, "failed to get replication info")
	}

	return info, nil
}

// set to master
func (c *Client) SetAsMaster(ip string, port int32, password string) error {
	rclient := c.initClient(ip, port, password)

	if err := rclient.SlaveOf(context.Background(), "NO", "ONE").Err(); err != nil {
		return errors.Wrap(err, "failed to set as master")
	}
	return nil
}

// set to slave
func (c *Client) SetAsSlave(slaveIP, masterIP string, port int32, password string) error {
	rclient := c.initClient(slaveIP, port, password)

	if err := rclient.SlaveOf(context.Background(), masterIP, strconv.Itoa(int(port))).Err(); err != nil {
		return errors.Wrap(err, "failed to set as slave")
	}

	return nil
}

// return result: masterIP, masterPort, error
func (c *Client) GetSentinelMonitor(sentinelIP string, password string) (string, string, error) {
	ctx := context.Background()
	rclient := c.initClientForSentinel(sentinelIP, password)

	monitorInfo, err := rclient.GetMasterAddrByName(ctx, "mymaster").Result()
	if err != nil {
		return "", "", errors.Wrap(err, "failed to get sentinel monitor info")
	}

	return monitorInfo[0], monitorInfo[1], nil
}

func (c *Client) SetSentinelMonitor(sentinelIP string, password string, monitor map[string]interface{}) error {
	ctx := context.Background()
	rclient := c.initClientForSentinel(sentinelIP, password)

	if err := rclient.Remove(ctx, "mymaster").Err(); err != nil {
		return errors.Wrap(err, "failed to remove monitoring master")
	}

	monitorIP := monitor["masterIP"].(string)
	monitorPort := monitor["port"].(int32)
	quoram := monitor["quorum"].(string)
	if err := rclient.Monitor(ctx, "mymaster", monitorIP, strconv.Itoa(int(monitorPort)), quoram).Err(); err != nil {
		return errors.Wrap(err, "faield to monitoring a new master")
	}

	if password != "" {
		if err := rclient.Set(ctx, "mymaster", "auth-pass", password).Err(); err != nil {
			return errors.Wrap(err, "failed to set sentinel auth-pass")
		}
	}

	return nil
}

func (c *Client) initClient(ip string, port int32, password string) *redis.Client {
	rClient := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", ip, port),
		Password: password,
	})

	return rClient
}

func (c *Client) initClientForSentinel(ip string, password string) *redis.SentinelClient {
	rClient := redis.NewSentinelClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", ip, 26379),
		Password: password,
	})

	return rClient
}

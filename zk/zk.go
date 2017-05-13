package zk

import (
	"path/filepath"
	"strings"
	"time"
	log "github.com/golang/glog"

	zk "github.com/samuel/go-zookeeper/zk"
)

// Client provides a wrapper around the zookeeper client
type Client struct {
	client *zk.Conn
}

func (c *Client) Close() {
	if c != nil && c.client != nil {
		c.client.Close()
	}
}

func NewClient(machines []string) (*Client, error) {
	c, _, err := zk.Connect(machines, time.Second) //*10)
	if err != nil {
		panic(err)
	}
	return &Client{c}, nil
}

func nodeWalk(prefix string, c *Client, vars map[string]string) error {
	l, stat, err := c.client.Children(prefix)
	if err != nil {
		return err
	}

	if stat.NumChildren > 0 {
		for _, key := range l {
			s := prefix + "/" + key
			_, stat, err := c.client.Exists(s)
			if err != nil {
				return err
			}
			if stat.NumChildren == 0 {
				b, _, err := c.client.Get(s)
				if err != nil {
					return err
				}
				vars[s] = string(b)
			} else {
				nodeWalk(s, c, vars)
			}
		}
	}
	return nil
}

// return subtree of these nodes
func (c *Client) GetValues(keys []string) (map[string]string, error) {
	vars := make(map[string]string)
	for _, v := range keys {
		if strings.HasSuffix(v, "/") {
			v = v + "/"
		}
		e, _, err := c.client.Exists(v)
		if err != nil {
			return vars, err
		}

		if !e {
			return nil, nil
		}

		if v == "/" {
			v = ""
		}
		err = nodeWalk(v, c, vars)
		if err != nil {
			return vars, err
		}
	}
	return vars, nil
}

func (c *Client) SetNodeValue(key string, value string) error {
	var err error
	key = strings.Replace(key, "/*", "", -1)
	_, _, err = c.client.Exists(key)
	if err != nil {
		return err
	}

	_, err = c.client.Set(key, ([]byte)(value), -1)

	if err != nil {
		return err
	}

	return nil
}

func (c *Client) GetNodeValue(key string) (string, error) {
	var err error
	var b []byte
	key = strings.Replace(key, "/*", "", -1)
	_, _, err = c.client.Exists(key)
	if err != nil {
		return "", err
	}

	b, _, err = c.client.Get(key)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (c *Client) GetNodeChildren(key string) ([]string, error) {
	var err error
	var ret []string
	key = strings.Replace(key, "/*", "", -1)
	_, _, err = c.client.Exists(key)
	if err != nil {
		return ret, err
	}

	ret, _, err = c.client.Children(key)
	if err != nil {
		return ret, err
	}
	return ret, nil
}

func (c *Client) GetNodesValues(keys []string) (map[string]string, error) {
	vars := make(map[string]string)
	for _, v := range keys {
		nv := strings.Replace(v, "/*", "", -1)
		_, _, err := c.client.Exists(nv)
		if err != nil {
			return vars, err
		}

		b, _, err := c.client.Get(nv)
		if err != nil {
			return vars, err
		}
		vars[v] = string(b)
	}
	return vars, nil
}

type watchResponse struct {
	waitIndex uint64
	err       error
}

func (c *Client) watch(key string, respChan chan watchResponse, cancelRoutine chan bool) {
	_, _, keyEventCh, err := c.client.GetW(key)
	if err != nil {
		respChan <- watchResponse{0, err}
	}
	_, _, childEventCh, err := c.client.ChildrenW(key)
	if err != nil {
		respChan <- watchResponse{0, err}
	}

	for {
		select {
		case e := <-keyEventCh:
			if e.Type == zk.EventNodeDataChanged {
				respChan <- watchResponse{1, e.Err}
			}
		case e := <-childEventCh:
			if e.Type == zk.EventNodeChildrenChanged {
				respChan <- watchResponse{1, e.Err}
			}
		case <-cancelRoutine:
			log.V(10).Info("Stop watching: " + key)
			// There is no way to stop GetW/ChildrenW so just quit
			return
		}
	}
}

func (c *Client) WatchPrefix(prefix string, keys []string, waitIndex uint64, stopChan chan bool) (uint64, error) {
	// return something > 0 to trigger a key retrieval from the store
	if waitIndex == 0 {
		return 1, nil
	}

	// List the childrens first
	entries, err := c.GetValues([]string{prefix})
	if err != nil {
		return 0, err
	}

	respChan := make(chan watchResponse)
	cancelRoutine := make(chan bool)
	defer close(cancelRoutine)

	//watch all subfolders for changes
	watchMap := make(map[string]string)
	for k, _ := range entries {
		for _, v := range keys {
			if strings.HasPrefix(k, v) {
				for dir := filepath.Dir(k); dir != "/"; dir = filepath.Dir(dir) {
					if _, ok := watchMap[dir]; !ok {
						watchMap[dir] = ""
						log.V(10).Info("Watching: " + dir)
						go c.watch(dir, respChan, cancelRoutine)
					}
				}
				break
			}
		}
	}

	//watch all keys in prefix for changes
	for k, _ := range entries {
		for _, v := range keys {
			if strings.HasPrefix(k, v) {
				log.V(10).Info("Watching: " + k)
				go c.watch(k, respChan, cancelRoutine)
				break
			}
		}
	}

	for {
		select {
		case <-stopChan:
			return waitIndex, nil
		case r := <-respChan:
			return r.waitIndex, r.err
		}
	}
}

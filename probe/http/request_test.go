package http

import (
	"context"
	"fmt"
	"io/ioutil"
	"testing"
	"time"
)

func TestRequest(t *testing.T) {
	ctx, cf := context.WithTimeout(context.Background(), 3*time.Second)
	defer cf()
	url := "http://sz.to8to.com/"

	resp, tt, err := Request(ctx, nil, "GET", url, nil, nil)

	time.Sleep(2 * time.Second)
	time.Sleep(200 * time.Millisecond)
	dat, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	fmt.Printf("dat: %s\n", dat)
	fmt.Printf("err: %v\n", err)

	t.Errorf("%v, %+v, %v", resp, tt, err)
}

package dial

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"strings"
	"time"

	"fmt"

	"golang.org/x/net/context"
)

var (
	GetConnT   time.Time
	GotConnT   time.Time
	ConnStartT time.Time
	ConnDoneT  time.Time
	ReqStartT  time.Time
	GotFByteT  time.Time
	DNSStartT  time.Time
	DNSDoneT   time.Time
)

//Drequest is a dial request messages
type Drequest struct {
	Header   map[string]string
	Body     []byte
	Method   string
	DailType string //http or exec or other
	URL      string
	Timeout  time.Duration
}

type Elapsed struct {
	DNStime      time.Duration
	CreateConn   time.Duration
	StartRequest time.Duration
	FirstByteR   time.Duration
	Totletime    time.Duration
}

//Dresponse is dialtest result
type Dresponse struct {
	Header  map[string]string
	Body    string
	Elapsed Elapsed
	Status  bool
	Result  string
}

type Op struct {
	client       *http.Client
	responsetype string
	Checkmethod  map[string]string
	//Jmode       string //json mode [rpc/platform/other]
	//WithResultContains, WithResultSize, WitherResultMd5sum, WithHttpClient
}

type Option func(*Op)

func WithResponsetype(responsetype string) Option {
	return func(op *Op) { op.responsetype = responsetype }
}

func WithClinet(client *http.Client) Option {
	return func(op *Op) { op.client = client }
}

func WithCheckMethod(method map[string]string) Option {
	return func(op *Op) {
		op.Checkmethod = method
	}
}

//Dial a service, with service and interface set in the header,and this type is http
//method:GET POST PUT DELETE..
func Dial(dreq *Drequest, options ...Option) (dresp *Dresponse, err error) {
	var reqJSON map[string]interface{}
	dresp = &Dresponse{}

	reopt := &Op{}
	for _, o := range options {
		o(reopt)
	}

	//if request Body is not json,then sucker we are,because only applicable to Java
	err = json.Unmarshal(dreq.Body, &reqJSON)
	if err != nil {
		return
	}

	var buf bytes.Buffer
	if err = json.NewEncoder(&buf).Encode(reqJSON); err != nil {
		return
	}

	if dreq.Timeout == 0 {
		dreq.Timeout = 5 * time.Second
	}

	if reopt.client != nil {
		reopt.client.Timeout = dreq.Timeout
	} else {
		reopt.client = &http.Client{Timeout: dreq.Timeout}
	}

	var resp *http.Response
	var reqtime Elapsed

	reqtime, resp, err = request(reopt.client, dreq.Method, dreq.URL, dreq.Header, &buf)

	if err != nil {
		return
	}

	defer resp.Body.Close()
	//start check dial result

	var out []byte
	out, err = ioutil.ReadAll(resp.Body)

	var result string
	if len(out) > 1024 {
		result = string(out[:1024])
	} else {
		result = string(out)
	}

	dresp.Header = dreq.Header
	dresp.Body = string(dreq.Body)
	dresp.Elapsed = reqtime
	dresp.Result = result

	respstatus := statuschk(resp.StatusCode, dreq.DailType, string(out), reopt.Checkmethod)
	dresp.Status = respstatus
	return
}

// client: 对java服务不需要， 对php可能需要先认证
func request(client *http.Client, method, url string, header map[string]string, body io.Reader) (Elapsed, *http.Response, error) {
	requestT := Elapsed{}

	traceCtx := httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{
		GetConn: func(hostPort string) {
			fmt.Printf("Prepare to get a connection for %s.\n", hostPort)
			GetConnT = time.Now()
		},
		GotConn: func(info httptrace.GotConnInfo) {
			fmt.Printf("Got a connection: reused: %v, from the idle pool: %v.\n", info.Reused, info.WasIdle)
			GotConnT = time.Now()
		},
		ConnectStart: func(network, addr string) {
			fmt.Printf("Dialing... (%s:%s).\n", network, addr)
			ConnStartT = time.Now()
		},
		ConnectDone: func(network, addr string, err error) {
			if err == nil {
				fmt.Printf("Dial is done. (%s:%s)\n", network, addr)
				ConnDoneT = time.Now()
				requestT.CreateConn = time.Since(GetConnT)
			} else {
				fmt.Printf("Dial is done with error: %s. (%s:%s)\n", err, network, addr)
			}
		},
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err == nil {
				//fmt.Println("Wrote a request: ok.")
				ReqStartT = time.Now()
				requestT.StartRequest = time.Since(GetConnT)
			} else {
				fmt.Println("Wrote a request:", info.Err.Error())
			}
		},
		GotFirstResponseByte: func() {
			//fmt.Println("Got the first response byte.")
			GotFByteT = time.Now()
			requestT.FirstByteR = time.Since(GetConnT)
		},
		DNSStart: func(info httptrace.DNSStartInfo) {
			//fmt.Println("DNS start.")
			DNSStartT = time.Now()
		},
		DNSDone: func(info httptrace.DNSDoneInfo) {
			//fmt.Println("DNS done.")
			DNSDoneT = time.Now()
			requestT.DNStime = time.Since(DNSStartT)
		},
	})

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return requestT, nil, err
	}

	if header != nil {
		for k, v := range header {
			req.Header.Set(k, v)
		}
	}
	starttime := time.Now()
	req = req.WithContext(traceCtx)
	resp, err := client.Do(req)
	requestT.Totletime = time.Since(starttime)
	return requestT, resp, err
}

// 具体怎么检测返回的结果，可以在调用的dial的时候以option的形式指定
// WithResultContains, WithResultSize, WitherResultMd5sum, WithHttpCode等
func statuschk(code int, dtype, response string, checkmethod map[string]string) bool {
	f := false
	if dtype == "http" {
		f = check(code, response, checkmethod)
	}
	return f
}

func check(code int, response string, checkmethod map[string]string) bool {
	s := false
	for k, v := range checkmethod {
		switch k {
		case "Exclude":
			s = chkInclude(response, v)
			if s {
				s = false
			} else {
				s = true
			}
		case "Include":
			s = chkInclude(response, v)
		case "Contains":
			s = chkContains(response, v)
		case "Md5":
			s, _ = chkMd5(response, v)
		case "Sizege":
			s = chkSize(response, v, "ge")
		case "Sizele":
			s = chkSize(response, v, "le")
		case "StatusCode":
			dcode, err := strconv.Atoi(v)
			if err == nil {
				s = chkCode(code, dcode)
			}
		default:
			s = false
		}

		if !s {
			break
		}
	}
	return s
}

func chkMd5(response, md5sum string) (bool, error) {
	//md5
	d := []byte(response)
	rmd5 := fmt.Sprintf("%x", md5.Sum(d))

	if md5sum == rmd5 {
		return true, nil
	}
	return false, nil
}

func chkSize(response, size, j string) bool {
	f := false
	reby, err := strconv.Atoi(size)
	if err != nil {
		return f
	}
	switch j {
	case "ge":
		if len(response) > reby {
			f = true
		}
	case "le":
		if len(response) < reby {
			f = true
		}
	default:
		f = false
	}
	return f
}

func chkContains(response, contain string) bool {
	f := false
	if strings.Contains(response, contain) {
		f = true
	}
	return f
}

func chkInclude(response, item string) bool {
	f := false
	var dat map[string]interface{}
	if err := json.Unmarshal([]byte(response), &dat); err == nil {
		if _, ok := dat[item]; ok {
			f = true
		}

	}
	return f
}

func chkCode(rcode, dcode int) bool {
	if rcode == dcode {
		return true
	}
	return false
}

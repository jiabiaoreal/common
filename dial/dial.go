package dial

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptrace"
	"strings"
	"time"

	"golang.org/x/net/context"
)

//Drequest is a dial request messages
type Drequest struct {
	Header   map[string]string
	Body     []byte
	Method   string //method must upper:GET POST PUT...
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
	//Header   map[string]string
	//Body     string
	Elapsed  Elapsed
	Result   string
	Response *http.Response
	Status   bool
}

type Op struct {
	client       *http.Client
	responsetype string
	responsemin  int64
	responsemax  int64
	responsemd5  string
	includeItem  string
	excludeItem  string
	contain      string
	code         int
	//Checkmethod  map[string]string
	//WithResultContains, WithResultSize, WithResultMd5sum, WithHttpClient
}

type Option func(*Op)

func WithResponsetype(responsetype string) Option {
	return func(op *Op) { op.responsetype = responsetype }
}

func WithHttpClinet(client *http.Client) Option {
	return func(op *Op) { op.client = client }
}

func WithResultCode(code int) Option {
	return func(op *Op) {
		op.code = code
	}
}

func WithResultSize(min, max int64) Option {
	return func(op *Op) {
		op.responsemin = min
		op.responsemax = max
	}
}

func WithResultInclude(item string) Option {
	return func(op *Op) {
		op.includeItem = item
	}
}

func WithResultExclude(item string) Option {
	return func(op *Op) {
		op.excludeItem = item
	}
}

func WithResultContains(contian string) Option {
	return func(op *Op) {
		op.contain = contian
	}
}

func WithResultMd5sum(md5 string) Option {
	return func(op *Op) {
		op.responsemd5 = md5
	}
}

//Dial a service, with service and interface set in the header,and this type is http
//method:GET POST PUT DELETE..
func Dial(dreq *Drequest, options ...Option) (res *Dresponse, err error) {
	res = &Dresponse{}

	reopt := &Op{}
	for _, o := range options {
		o(reopt)
	}

	if dreq.Timeout == 0 {
		dreq.Timeout = 5 * time.Second
	}

	if reopt.client != nil {
		reopt.client.Timeout = dreq.Timeout
	} else {
		reopt.client = &http.Client{Timeout: dreq.Timeout}
	}

	//var resp *http.Response
	//var reqtime Elapsed

	reqByte := bytes.NewReader(dreq.Body)
	res, err = request(reopt.client, dreq.Method, dreq.URL, dreq.Header, reqByte)

	if err != nil {
		return
	}

	defer res.Response.Body.Close()

	/*r := strings.NewReader("some io.Reader stream to be read\n")
	  	lr := io.LimitReader(resp.Body, 1024*1024)

	  if _, err := io.Copy(os.Stdout, lr); err != nil {
	      log.Fatal(err)
	  }*/
	lr := io.LimitReader(res.Response.Body, 1024*1024*10)
	var out []byte
	out, err = ioutil.ReadAll(lr)
	res.Result = string(out)

	//var result string
	if len(out) >= 1024*1024*10-1 {
		//	result = string(out[:1024*1024*10])
		res.Status = false
	} else {
		//result = string(out)
		res.Status = statuschk(res.Response.StatusCode, dreq.DailType, string(out), reopt)
	}
	return
}

// client: 对java服务不需要， 对php可能需要先认证
func request(client *http.Client, method, url string, header map[string]string, body io.Reader) (*Dresponse, error) {
	res := &Dresponse{}
	var tracetime struct {
		connstart    time.Time
		connDone     time.Time
		DNSstart     time.Time
		DNSdone      time.Time
		getconn      time.Time
		gotconn      time.Time
		gotfbyte     time.Time
		requeststart time.Time
	}

	traceCtx := httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{
		GetConn: func(hostPort string) {
			fmt.Printf("Prepare to get a connection for %s.\n", hostPort)
			tracetime.getconn = time.Now()
		},
		GotConn: func(info httptrace.GotConnInfo) {
			fmt.Printf("Got a connection: reused: %v, from the idle pool: %v.\n", info.Reused, info.WasIdle)
			tracetime.gotconn = time.Now()
		},
		ConnectStart: func(network, addr string) {
			fmt.Printf("Dialing... (%s:%s).\n", network, addr)
			tracetime.connstart = time.Now()
		},
		ConnectDone: func(network, addr string, err error) {
			if err == nil {
				fmt.Printf("Dial is done. (%s:%s)\n", network, addr)
				tracetime.connDone = time.Now()
				res.Elapsed.CreateConn = time.Since(tracetime.getconn)
			} else {
				fmt.Printf("Dial is done with error: %s. (%s:%s)\n", err, network, addr)
			}
		},
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err == nil {
				tracetime.requeststart = time.Now()
				res.Elapsed.StartRequest = time.Since(tracetime.getconn)
			} else {
				fmt.Println("Wrote a request:", info.Err.Error())
			}
		},
		GotFirstResponseByte: func() {
			//fmt.Println("Got the first response byte.")
			tracetime.gotfbyte = time.Now()
			res.Elapsed.FirstByteR = time.Since(tracetime.getconn)
		},
		DNSStart: func(info httptrace.DNSStartInfo) {
			//fmt.Println("DNS start.")
			tracetime.DNSstart = time.Now()
		},
		DNSDone: func(info httptrace.DNSDoneInfo) {
			//fmt.Println("DNS done.")
			tracetime.DNSdone = time.Now()
			res.Elapsed.DNStime = time.Since(tracetime.DNSstart)
		},
	})

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return res, err
	}

	if header != nil {
		for k, v := range header {
			req.Header.Set(k, v)
		}
	}

	req = req.WithContext(traceCtx)
	starttime := time.Now()
	resp, err := client.Do(req)
	res.Elapsed.Totletime = time.Since(starttime)
	//res.Elapsed = requestT
	res.Response = resp

	return res, err
}

// 具体怎么检测返回的结果，可以在调用的dial的时候以option的形式指定
// WithResultContains, WithResultSize, WitherResultMd5sum, WithHttpCode等
func statuschk(code int, dailtype, response string, op *Op) bool {
	f := true
	flag := true

	if op.code != 0 {
		f = chkCode(code, op.code)
		if f == false {
			return f
		}
		flag = false
	}

	if op.contain != "" {
		f = chkContains(response, op.contain)
		if f == false {
			return f
		}
		flag = false
	}

	if op.excludeItem != "" {
		f = chkInclude(response, op.excludeItem)
		if f {
			f = false
			return f
		}
		flag = false
	}

	if op.includeItem != "" {
		f = chkInclude(response, op.includeItem)
		flag = false
		if f == false {
			return f
		}
	}

	if op.responsemd5 != "" {
		f, _ = chkMd5(response, op.responsemd5)
		flag = false
		if f == false {
			return f
		}
	}

	if op.responsemin != 0 {
		f = chkSize(response, op.responsemin, "ge")
		if f == false {
			return f
		}
		flag = false
	}

	if op.responsemax != 0 {
		f = chkSize(response, op.responsemax, "le")
		if f == false {
			return f
		}
		flag = false
	}

	//if flag is true,no checkmethod give use default checkmethod
	if flag {
		if dailtype == "http" {
			if code >= 400 {
				f = false
			}
		}
	}

	return f
}

/*func check(code int, response string, checkmethod map[string]string) bool {
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
			//s = chkSize(response, v, "ge")
		case "Sizele":
			//s = chkSize(response, v, "le")
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
}*/

func chkMd5(response, md5sum string) (bool, error) {
	//md5
	d := []byte(response)
	rmd5 := fmt.Sprintf("%x", md5.Sum(d))

	if md5sum == rmd5 {
		return true, nil
	}
	return false, nil
}

func chkSize(response string, size int64, j string) bool {
	f := false
	switch j {
	case "ge":
		if int64(len(response)) > size {
			f = true
		}
	case "le":
		if int64(len(response)) < size {
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

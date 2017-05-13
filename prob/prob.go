package prob

type ProbType string

const (
	ProbHttp ProbType = "http"
	ProbTcp  ProbType = "tcp"
)

type ProbChecker func(data []byte, header map[string]string) bool
type ProbConfig struct {
	Type    ProbType
	Header  map[string]string
	Payload []byte
	Checker ProbChecker
}

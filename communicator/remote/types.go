package remote

type ConnInfo map[string]string

// UIOutput is the interface that must be implemented to output
// data to the end user.
type UIOutput interface {
	Output(string)
}

type ResourceConfig struct {
	Raw    map[string]interface{}
	Config map[string]interface{}
}

package extension

type Extension interface {
	Name() string
	Components() []string
}

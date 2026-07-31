package server

type Instance struct {
	Name string
	Port int
	DNS  string
}

var Instances = []Instance{
	{
		Name: "AlamamaPal01",
		Port: 8211,
		DNS:  "",
	},
	{
		Name: "KalagaPal01",
		Port: 8212,
		DNS:  "",
	},
}

# Docker Registry Go lib
This is a simple Go package to use with the Docker Registry v1.

# Example

```
import registry "github.com/ehazlett/orca/registry/v1"

// make sure to handle the err
client, _ := registry.NewRegistryClient("http://localhost:5000", nil)

res, _ := client.Search("busybox", 1, 100)

fmt.Printf("Number of Repositories: %d\n", res.NumberOfResults)
for _, r := range res.Results {
	fmt.Printf(" -  Name: %s\n", r.Name)
}
```

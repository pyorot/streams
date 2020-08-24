package utils

// Await : object to await one or many async tasks
// can't be used if the tasks return something
// (will fix this once Go gets generics)
// a task should be created and awaited in the same thread
type Await []chan (bool)

// Add : add task to await object
func (a *Await) Add(ch chan (bool)) {
	*a = append(*a, ch)
}

// Flush : await every task in await object, then clear
func (a *Await) Flush() {
	for _, ch := range *a {
		<-ch
	}
	*a = nil
}

package engine

import (
	"fmt"
)

type workflow struct {
	id          uint64
	name        string
	description string
	payload     string
	steps       []WorkflowStep
}

func (w *workflow) addWorkflowStep(s WorkflowStep) {
	w.steps = append(w.steps, s)
}

func (w *workflow) run(payload string, e *engine) {
	fmt.Printf("\nRunning workflow #%d\n", w.id)
	w.payload = payload
	var err error
	// loop through all the steps inside of this workflow
	for _, s := range w.steps {
		w.payload, err = s.run(w.payload, e) // overwrite payload with each step execution and keep on passing this payload to each step
		if err != nil {
			// For now, we don't halt the workflow if a step encounters an error
			e.log(fmt.Sprintf("Workflow Step execution error: WorkflowID:%d Error:%s", w.id, err))
		}
	}
}

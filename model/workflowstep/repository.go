package workflowstep

// Repository facilitates persistence and retrieval of workflow steps.
type Repository interface {
	Save(workflowStep *WorkflowStep) error
}

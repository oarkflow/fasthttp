package workflow

import "github.com/oarkflow/fh"

type Workflow struct {
	Name  string
	Steps []Step
}
type Step struct {
	Name      string
	Handler   fh.HandlerFunc
	JobType   string
	Condition func(*fh.Ctx) bool
}

func New(name string) *Workflow { return &Workflow{Name: name} }
func (w *Workflow) Use(name string, handler fh.HandlerFunc) *Workflow {
	w.Steps = append(w.Steps, Step{Name: name, Handler: handler})
	return w
}
func (w *Workflow) Job(name, jobType string) *Workflow {
	w.Steps = append(w.Steps, Step{Name: name, JobType: jobType})
	return w
}
func (w *Workflow) Handler() fh.HandlerFunc {
	return func(c *fh.Ctx) error {
		for _, step := range w.Steps {
			if step.Condition != nil && !step.Condition(c) {
				continue
			}
			c.Lifecycle().Mark(c, fh.LifecycleProcessing)
			if step.Handler != nil {
				if err := step.Handler(c); err != nil {
					return err
				}
			}
			if step.JobType != "" {
				id, err := fh.AtomicHandoff(c, step.JobType, fh.Map{"workflow": w.Name, "step": step.Name, "request_id": c.Locals("request_id")})
				if err != nil {
					return err
				}
				c.Locals("job_id", id)
			}
		}
		c.Lifecycle().Mark(c, fh.LifecycleCompleted)
		return nil
	}
}

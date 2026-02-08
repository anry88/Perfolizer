package core

// Project holds multiple test plans and is the top-level entity for save/load.
type Project struct {
	Name  string
	Plans []PlanEntry
}

// PlanEntry is a named test plan (root element) inside a project.
type PlanEntry struct {
	Name string
	Root TestElement
}

// NewProject creates a project with the given name and no plans.
func NewProject(name string) *Project {
	return &Project{Name: name, Plans: make([]PlanEntry, 0)}
}

// AddPlan appends a new plan to the project.
func (p *Project) AddPlan(name string, root TestElement) {
	p.Plans = append(p.Plans, PlanEntry{Name: name, Root: root})
}

// RemovePlanAt removes the plan at the given index. Does nothing if index is out of range.
func (p *Project) RemovePlanAt(index int) {
	if index < 0 || index >= len(p.Plans) {
		return
	}
	p.Plans = append(p.Plans[:index], p.Plans[index+1:]...)
}

// PlanCount returns the number of plans in the project.
func (p *Project) PlanCount() int {
	return len(p.Plans)
}

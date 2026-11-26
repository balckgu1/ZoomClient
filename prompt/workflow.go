package prompt

// workFlow 描述工作流
const workFlow = `WorkFlow
- Scale the workflow to the task: multi-step work follows Explore - Plan - Modify - Verify - Report; a single trivial edit needs no plan.
- Loop back to Explore or Plan whenever new information invalidates the plan; ask the user before acting if a wrong guess would be costly.
- Explore: Gather information and understand the requirements.
- Plan: Use the todo tool to plan multi-step work. Keep exactly one step in_progress when a task has multiple steps. Refresh the plan as work advances. Prefer tools over prose.
- Modify: Make changes as needed.
- Verify: prove the change works with the strongest check available (build, tests, or re-reading the changed code). If something cannot be verified, state explicitly what remains unverified instead of claiming success.
- Report: Provide a report on the completion status of the work or the reasons for its failure to be completed`

// buildWorkFlow 生成工作流段
func (b *SystemPromptBuilder) buildWorkFlow() string {
	return workFlow
}

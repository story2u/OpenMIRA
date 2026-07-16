export function validateJobAnalysisContext(input, result) {
  if (!input.job_discovery && result.job_analysis !== null) {
    throw new Error('job_analysis requires job_discovery context')
  }
  const analysis = result.job_analysis
  if (!analysis) return
  const formal = analysis.classification === 'job_post' || analysis.classification === 'job_repost'
  if (formal !== (analysis.job !== null)) {
    throw new Error('formal job classifications must include job and noise classifications must not')
  }
  if (!analysis.job) return
  if (!analysis.field_evidence.job_title) throw new Error('missing field evidence for job_title')
  if (analysis.job.salary.raw && !analysis.field_evidence.salary) {
    throw new Error('missing field evidence for salary')
  }
}

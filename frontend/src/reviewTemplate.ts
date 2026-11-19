/** Default PR review prompt template (shared by Settings and the PR details panel). */
export const DEFAULT_REVIEW_TEMPLATE = `You are an expert code reviewer. Please review the following pull request carefully.

## Instructions
- Identify bugs, security issues, and logic errors.
- Suggest improvements to code clarity and maintainability.
- Check for missing tests or documentation.
- Be concise and prioritize critical issues.
- Note any positive aspects of the implementation.

## Historical Reviewer Rules
{{REVIEW_RULES}}

## PR Title
{{PR_TITLE}}

## PR Description
{{PR_DESCRIPTION}}

## Code Changes
{{PR_DIFF}}`;

/** Placeholders understood by the prompt builder. */
export const TEMPLATE_PLACEHOLDERS = ['{{PR_TITLE}}', '{{PR_DESCRIPTION}}', '{{PR_DIFF}}', '{{REVIEW_RULES}}'];

# Issue tracker: GitHub

Issues and specs for this repository live in GitHub Issues at
`TheGeeKing/FileGot`. Use the `gh` CLI for operations.

## Conventions

- Create: `gh issue create --title "..." --body "..."`
- Read: `gh issue view <number> --comments`
- List: `gh issue list --state open`
- Comment: `gh issue comment <number> --body "..."`
- Label: `gh issue edit <number> --add-label "..."`
- Close: `gh issue close <number> --comment "..."`
- Pull requests are not treated as incoming feature requests.

## Skill operations

When a skill says "publish to the issue tracker," create a GitHub issue.
When it says "fetch the relevant ticket," read that issue and its comments.

## Planning

Large efforts use one map issue with linked child issues. Child issues declare
blocking dependencies using GitHub issue dependencies where available, with a
`Blocked by: #<number>` fallback.

An issue is ready when all blockers are closed and it is unassigned.

# Engineering Operating System Strategy

Date: 2026-06-01  
Working thesis: eliminate TPM/program-manager coordination labor by giving software engineers a trusted, evidence-backed operating system for deciding what to do next.

This document is intentionally expansive. Treat it as seed context for product, architecture, roadmap, and implementation.

## 0. One-Line Thesis

```text
Turn engineering activity into an operational graph that tells engineers:
what changed, what is blocked, what matters, who owns it, what decision is pending,
and what exact action moves the system forward.
```

This is not a better summary tool. A summary tool still leaves coordination work to humans. The target product absorbs the coordination loop itself.

## 1. The Real Job To Replace

TPMs and program managers usually do this invisible work:

```text
raw signals
  |
  +-- collect status
  +-- track dependencies
  +-- detect blockers
  +-- chase owners
  +-- remember decisions
  +-- translate meetings into actions
  +-- reconcile roadmap vs reality
  +-- create stakeholder updates
  +-- protect delivery focus
  +-- surface risks before they explode
```

The product must turn those into software behaviors:

```text
status collection      -> automatic evidence graph
dependency tracking    -> typed edges and stale handoff detection
blocker detection      -> state machines + graph inference
owner chasing          -> action queue + reminders + writeback
decision memory        -> decision objects linked to evidence
roadmap reconciliation -> goal/milestone/objective objects
stakeholder updates    -> generated from source-of-truth evidence
risk surfacing         -> risk objects with trend and blast radius
focus protection       -> engineer-specific cockpit, not meeting churn
```

## 2. Product North Star

The engineer opens the product and sees:

```text
1. What changed since I last looked.
2. What blocks my current work.
3. What I should do next.
4. What decision needs me.
5. Which PR/issue/build/doc/message matters.
6. Who owns adjacent work.
7. Which risk is growing.
8. What the system will write/update on my behalf.
```

The output should feel less like:

```text
Here is a summary of your project.
```

and more like:

```text
You are blocked on API contract decision D-42.

Evidence:
- PR #184 changed the request schema.
- Issue ENG-92 still assumes the old field.
- CI failed in service-b integration tests.
- Priya asked for confirmation yesterday.

Recommended next action:
Ask Priya to confirm schema by 3pm or merge the compatibility shim.

Safe writeback:
Draft comment on PR #184 and update ENG-92 blocker field.
```

## 3. Core Product DAG

```text
GitHub / Jira / Linear / CI / Docs / Webex / Slack / Calendar / Incidents
        |
        v
append-only raw event log
        |
        v
normalized engineering objects
        |
        v
typed engineering graph
        |
        v
state machines
        |
        +-- issue state
        +-- PR state
        +-- build/deploy state
        +-- dependency state
        +-- decision state
        +-- risk state
        +-- action state
        |
        v
reasoning layer
        |
        +-- blocker detector
        +-- dependency mapper
        +-- decision extractor
        +-- risk analyst
        +-- next-action planner
        +-- status writer
        |
        v
engineer cockpit + project map + writeback actions
```

## 4. Lessons From Glean

Official docs:

- Knowledge Graph: https://docs.glean.com/security/knowledge-graph
- Connectors: https://docs.glean.com/connectors/about
- Actions: https://docs.glean.com/agents/actions/introduction-to-actions
- Actions administration: https://docs.glean.com/administration/actions/home

Glean pattern:

```text
content + people + activity
        |
        v
permission-aware enterprise knowledge graph
        |
        v
personalized search / assistant / actions
```

What matters for this product:

```text
content:
  docs, issues, PRs, comments, messages, code, design specs, incidents

people:
  engineers, reviewers, owners, teams, collaborators, decision makers

activity:
  commits, reviews, status changes, meetings, comments, deploys, failures

permissions:
  every result/action must respect source-system access

connectors:
  source-specific crawlers must understand native APIs and permission maps

actions:
  read/reason/write loops are a separate layer from indexing
```

Key takeaways:

```text
1. Connectors are product infrastructure, not plumbing.
2. Permissions are part of the graph, not an afterthought.
3. People/activity signals are required for relevance.
4. Search alone is insufficient; actions are where coordination disappears.
```

Where to go beyond Glean:

```text
Glean optimizes enterprise knowledge retrieval.
This product must optimize engineering execution.

Search answers:
  "What do we know?"

Engineering OS answers:
  "What should happen next, and can I safely trigger it?"
```

## 5. Lessons From Palantir

Official docs:

- Ontology overview: https://www.palantir.com/docs/foundry/ontology/overview/
- Ontology system: https://www.palantir.com/docs/foundry/architecture-center/ontology-system/
- Ontologies: https://www.palantir.com/docs/foundry/ontologies/ontologies-overview/
- Functions on objects: https://www.palantir.com/docs/foundry/functions/functions-on-objects/
- Functions/actions: https://www.palantir.com/docs/foundry/functions/use-functions

Palantir pattern:

```text
data sources
    |
    v
ontology objects + properties + links
    |
    v
functions + actions + dynamic security
    |
    v
operational decisions and writeback
```

What matters for this product:

```text
semantic layer:
  Engineering nouns: PR, Issue, Service, Milestone, Incident, Decision, Risk.

kinetic layer:
  Engineering verbs: assign, request review, merge, rollback, update issue,
  publish status, create action, escalate blocker.

security layer:
  Different users can see/run different objects/actions.

logic layer:
  Rules, functions, LLM reasoning, and workflow orchestration all attach to objects.
```

Key takeaways:

```text
1. A real ontology models decisions, not just data.
2. Nouns need verbs.
3. Actions must be safe, auditable, and permissioned.
4. AI becomes useful when grounded in object state and writeback controls.
```

Where to simplify:

```text
Do not build a generic enterprise ontology platform first.
Build the smallest engineering ontology that eliminates coordination work.
```

## 6. Measurement Lessons: DORA And SPACE

Official/research docs:

- DORA metrics: https://dora.dev/guides/dora-metrics/
- SPACE framework: https://queue.acm.org/detail.cfm?id=3454124

DORA gives delivery loop health:

```text
change lead time
deployment frequency
change failure rate
failed deployment recovery time
deployment rework rate
```

SPACE warns against one-dimensional developer productivity metrics:

```text
S: satisfaction and well-being
P: performance
A: activity
C: communication and collaboration
E: efficiency and flow
```

Product implication:

```text
Do not rank engineers.
Do not optimize for raw activity.
Do not build surveillance.

Optimize coordination outcomes:
  decision latency
  blocker age
  stale dependency count
  PR review latency
  handoff latency
  rework from missed context
  time to understand project state
  interruption load
```

## 7. Target Ontology

### 7.1 Core Objects

```text
Engineer
Team
Repo
Service
Component
File
API
Project
Milestone
Objective
Issue
PR
Commit
Review
Build
TestFailure
Deploy
Incident
Doc
Meeting
Message
Decision
Risk
Blocker
Dependency
Question
Action
CustomerSignal
```

### 7.2 Core Edges

```text
Engineer member_of Team
Engineer owns Service
Team owns Repo
Repo contains Component
Component contains File
Service depends_on Service

Project has Milestone
Milestone tracks Issue
Issue belongs_to Project
Issue blocked_by Blocker
Issue depends_on Issue

PR implements Issue
PR changes File
PR reviewed_by Engineer
PR blocked_by Build
Commit belongs_to PR
Build validates Commit
Deploy releases Commit

Incident caused_by Deploy
Incident impacts Service
Risk threatens Milestone
Risk evidenced_by Event

Decision resolves Question
Decision affects Service
Decision captured_from Meeting
Decision evidenced_by Doc/Message/PR

Action assigned_to Engineer
Action resolves Blocker
Action updates Issue
Action comments_on PR
```

### 7.3 Object State Machines

PR state:

```text
draft -> ready -> waiting_review -> changes_requested -> approved -> mergeable -> merged
       \                                                        /
        +---------------- blocked_by_ci / blocked_by_decision--+
```

Issue state:

```text
backlog -> ready -> in_progress -> blocked -> review -> done
             |          |
             v          v
        needs_decision  needs_dependency
```

Decision state:

```text
proposed -> needs_owner -> under_discussion -> decided -> communicated -> superseded
```

Risk state:

```text
signal -> candidate -> active -> mitigated -> accepted -> realized
```

Action state:

```text
suggested -> approved -> executed -> verified -> closed
           \                      /
            +---- rejected -------+
```

## 8. Trust Contract

Every generated claim must carry:

```text
claim
evidence refs
confidence
why now
blast radius
owner
recommended action
writeback target
reversibility
permission check
audit record
```

Trust UI should always answer:

```text
Why am I seeing this?
What source proves it?
What happens if I accept?
Can I undo it?
Who else will see it?
```

## 9. Product Surfaces

### 9.1 Engineer Today

```text
What changed
What blocks me
What needs my review
What decision needs me
What action is safest next
```

### 9.2 Project Map

```text
goal -> milestones -> issues -> PRs -> builds -> deploys
       |
       +-- risks
       +-- blockers
       +-- decisions
       +-- dependencies
```

### 9.3 PR Intelligence

```text
PR
 |
 +-- linked issue
 +-- touched services
 +-- risky files
 +-- prior incidents touching same area
 +-- missing tests
 +-- reviewer fit
 +-- likely unblock action
```

### 9.4 Decision Inbox

```text
unresolved question
       |
       +-- evidence
       +-- options
       +-- owner
       +-- deadline pressure
       +-- affected work
       +-- recommended decision path
```

### 9.5 Dependency Radar

```text
Team A issue
     |
     v
blocking edge
     |
     v
Team B PR / decision / deploy
     |
     v
stale handoff alert
```

### 9.6 Auto Status

```text
project graph
   |
   v
evidence-backed update
   |
   +-- shipped
   +-- changed
   +-- blocked
   +-- risk
   +-- next action
```

### 9.7 Action Center

```text
recommended action
     |
     +-- draft issue update
     +-- draft PR comment
     +-- request review
     +-- create blocker
     +-- post status
     +-- schedule follow-up
```

## 10. Reasoning Agents

These should be internal services/jobs, not necessarily user-facing "agents."

```text
IngestionNormalizer
  Converts raw API payloads into normalized objects/events.

EntityResolver
  Resolves users, teams, repos, services, and duplicate references.

DependencyMapper
  Infers and validates depends_on / blocked_by edges.

DecisionExtractor
  Finds decision candidates in PRs, docs, issues, and meetings.

BlockerDetector
  Detects stale work, missing owner, failed CI loops, review starvation.

RiskAnalyst
  Scores delivery risk from dependency depth, churn, incidents, and ambiguity.

NextActionPlanner
  Proposes one small action that reduces uncertainty or unblocks work.

StatusWriter
  Generates stakeholder updates from graph evidence.

WritebackExecutor
  Runs approved actions against GitHub/Jira/Slack/etc.

OutcomeLearner
  Compares recommended actions to actual outcomes and updates scoring.
```

## 11. Data Architecture

### 11.1 Minimum Schema

```text
raw_events(
  event_id,
  source,
  source_id,
  event_type,
  occurred_at,
  actor_key,
  payload_json,
  payload_hash,
  ingested_at
)

entities(
  entity_id,
  entity_type,
  canonical_key,
  label,
  source,
  source_url,
  properties_json,
  created_at,
  updated_at
)

edges(
  edge_id,
  source_entity_id,
  relation,
  target_entity_id,
  evidence_event_id,
  confidence,
  created_at,
  updated_at
)

claims(
  claim_id,
  scope_entity_id,
  claim_type,
  statement,
  confidence,
  lifecycle,
  created_at,
  updated_at
)

claim_evidence(
  claim_id,
  evidence_event_id,
  stance,
  weight
)

actions(
  action_id,
  action_type,
  target_entity_id,
  proposed_by,
  status,
  payload_json,
  reversible,
  audit_json,
  created_at,
  updated_at
)
```

### 11.2 Retrieval Layers

```text
exact lookup:
  entity by source ID, PR number, issue key, service name

graph traversal:
  dependency path, owner path, risk blast radius

keyword:
  FTS5 / search index over docs, comments, messages

semantic:
  embeddings over docs/fragments/decisions/issues

reasoning context:
  assembled evidence packet with citations and graph neighborhood
```

## 12. Connector Roadmap

```text
P0 GitHub:
  repos, PRs, commits, reviews, comments, checks, codeowners

P0 Issue tracker:
  Jira/Linear issues, epics, projects, status, owners, labels

P1 CI/CD:
  workflows, builds, tests, deploys, flaky failures

P1 Docs:
  Google Docs/Notion/Confluence/Markdown specs, RFCs, design docs

P1 Chat/meeting:
  Slack/Webex/Teams threads, decisions, blockers, meeting notes

P2 Incident/observability:
  incidents, alerts, SLOs, deploy correlations

P2 Calendar:
  recurring syncs, decision meetings, stakeholder reviews
```

## 13. MVP That Can Actually Win

Do not start with "replace all PMs." Start with one painful loop:

```text
PR / issue / blocker loop
```

MVP:

```text
1. Ingest GitHub PRs, reviews, comments, checks.
2. Ingest Jira/Linear issues.
3. Link PRs to issues.
4. Detect:
   - stale PRs
   - blocked issues
   - failed CI loops
   - missing reviewers
   - review/comment unanswered
   - issue status inconsistent with PR reality
5. Generate:
   - daily engineer cockpit
   - project risk map
   - draft status update
   - draft PR/issue comments
6. Require approval before writeback.
```

MVP diagram:

```text
GitHub + Jira
    |
    v
PR/Issue graph
    |
    v
blocker + stale-state detection
    |
    v
next action
    |
    v
approved writeback
```

## 14. Current Cubicle Gap Analysis

Cubicle today:

```text
Webex messages -> SQLite messages
Webex messages -> belief_evidence
messages -> focus cache JSON
focus cache -> Codex summaries/topics/questions
belief_evidence -> Codex beliefs
```

Current strengths:

```text
SQLite persistence exists
schema migrations exist
belief lifecycle exists
manual belief override exists
Codex orchestration exists
question candidate table exists
focus cache reuse exists
runtime-root separation exists
```

Current gaps for Engineering OS:

```text
no GitHub/Jira ontology
no typed edge table
no normalized evidence joins
no action/writeback layer
no permission graph
no service ownership model
no project/milestone/risk/decision state machines
focus cache is flattened JSON projection
topics are persisted but not clearly consumed
transcripts do not write belief_evidence
most messages are not mirrored as webex_message evidence
```

Immediate codebase upgrade path:

```text
P0
  backfill messages -> belief_evidence
  transcript submit -> belief_evidence

P1
  add knowledge_edges
  add belief_evidence_links
  add question_evidence_links

P2
  add entities/events generic tables
  map rooms/people/messages into entities/events

P3
  add GitHub connector
  add Issue connector

P4
  add Action model with approved writeback
```

## 15. What The Product Must Not Become

```text
not a dashboard graveyard
not a generic chatbot
not a surveillance tool
not a summary generator
not a manager-reporting layer only
not an opaque AI oracle
not a second system of record with no writeback
```

If engineers do not trust it, they will ignore it.

## 16. Trust And Adoption Rules

```text
1. Engineers must be able to correct the graph.
2. Every insight must cite evidence.
3. Every action must be inspectable before execution.
4. The system must reduce meetings, not create review chores.
5. It must help the engineer first, leadership second.
6. Do not expose individual productivity rankings.
7. Prefer team/system bottlenecks over personal blame.
8. Learn from accepted/rejected recommendations.
```

## 17. Differentiation

Compared to enterprise search:

```text
Search finds information.
This decides the next engineering move.
```

Compared to project dashboards:

```text
Dashboards show state.
This explains causality, risk, and action.
```

Compared to AI coding assistants:

```text
Coding assistants help write code.
This coordinates the engineering system around the code.
```

Compared to TPMs:

```text
TPMs manually maintain a mental graph.
This product persists the graph, reasons over it, and executes approved actions.
```

## 18. Concrete Example

Input facts:

```text
ENG-102 is marked "In Progress".
PR #811 implements ENG-102.
PR #811 has failed integration tests for 2 days.
Review requested from Maya but no response after 36 hours.
PR #790 changed same API last week.
Design doc says schema freeze was expected by May 30.
Slack thread contains "we need Alex to decide field naming".
```

Derived graph:

```text
ENG-102 blocked_by CI failure
ENG-102 blocked_by decision D-17
PR #811 depends_on PR #790
D-17 assigned_to Alex
Risk R-33 threatens Milestone M-4
```

Engineer output:

```text
Next action:
Ask Alex to decide field naming, then update PR #811 schema fixture.

Why:
The issue is stale because CI is failing and the unresolved field-name decision
blocks both test repair and reviewer confidence.

Safe writeback:
1. Comment on PR #811 with failing check + decision link.
2. Set ENG-102 status to Blocked.
3. Create decision D-17 assigned to Alex.
```

## 19. First 30 Days Build Plan

Week 1:

```text
add generic event/entity/edge schema
add evidence join tables
fix transcript evidence write
backfill message evidence
add graph inspection CLI
```

Week 2:

```text
GitHub connector read-only
ingest PRs, commits, reviews, checks
map PR objects and edges
basic PR stale detector
```

Week 3:

```text
Issue tracker connector read-only
link PRs to issues
issue state reconciliation
blocker detector
engineer daily cockpit text view
```

Week 4:

```text
decision extractor
risk scoring v0
status writer v0
approval-gated writeback for PR comments and issue updates
```

## 20. First 90 Days Build Plan

```text
Month 1:
  core graph + GitHub/issues + stale/blocker insights

Month 2:
  docs/chat connectors + decisions + dependency map

Month 3:
  action center + permissions/audit + team cockpit + outcome learning
```

By day 90, the product should be able to replace these rituals for one team:

```text
daily status scraping
weekly project update drafting
manual stale PR chasing
manual blocker rollups
manual dependency tracking
manual decision log maintenance
```

## 21. Success Metrics

Do not optimize for "AI outputs generated."

Optimize:

```text
median time to understand project state
median blocker age
stale PR count
review wait time
decision latency
handoff latency
issue status accuracy
status-update prep time
meeting hours replaced
accepted recommendation rate
writeback success rate
reopened/rework rate from missed context
engineer-reported interruption load
```

## 22. Open Product Questions

```text
1. Is the first buyer an engineering leader or an individual engineer?
2. Is the first wedge GitHub/Jira, or Webex/Slack meeting intelligence?
3. Should writeback be human-approved forever, or graduate to policy-based autonomy?
4. What is the minimum permission model for team use?
5. Which action types are safe enough for v1?
6. How much ontology should users configure vs the system infer?
7. How do engineers correct wrong graph edges quickly?
8. What should be local-first vs cloud-hosted?
```

## 23. Recommended Positioning

Strong positioning:

```text
The Engineering Operating System that removes coordination drag.
```

Alternative:

```text
An execution graph for software teams.
```

Avoid:

```text
AI project manager
AI TPM
AI dashboard
enterprise search for engineering
```

Reason: engineers do not want another manager-shaped tool. They want fewer interruptions and better context.

## 24. Bottom Line

```text
Glean shows the importance of content + people + activity + permissions.
Palantir shows the importance of ontology + actions + security.
DORA shows how to measure delivery health.
SPACE shows how not to misuse productivity metrics.

Your product should combine those into:

  engineering objects
  typed dependencies
  evidence-backed reasoning
  safe actions
  outcome learning

The win condition is not better reporting.
The win condition is fewer humans needed to coordinate software delivery.
```


# Knowledge Graph Diagrams

Diagram-first view. Read this before the longer deep dive.

## 1. Current System

```text
              +----------------+
              | Config Targets |
              +-------+--------+
                      |
                      v
+---------+     +------------+      +----------------+
| Webex   +---->| messages   +----->| focus cache    |
+---------+     +-----+------+      | JSON snapshots |
                    |               +--------+-------+
                    |                        |
                    v                        v
              +-------------+        +---------------+
              | evidence    |        | Codex summary |
              +------+------+        | topics/titles |
                     |               +-------+-------+
                     v                       |
              +-------------+                v
              | beliefs     |          +----------+
              +-------------+          | topics   |
                                       +----------+

+-------------+       +----------+       +---------------------+
| transcripts |------>| messages |--x--->| belief evidence     |
+-------------+       +----------+       | currently missing   |
                                         +---------------------+

+----------+       +-------------+       +---------------------+
| iMessage |------>| focus cache |------>| questions pipeline  |
+----------+       +-------------+       +---------------------+
```

## 2. SQLite Shape

```text
rooms
  |
  +-- messages
  |     |
  |     +-- files
  |
  +-- belief_evidence

people
  |
  +-- messages.person_id
  |
  +-- belief_evidence.person_id

belief_evidence
  |
  v
beliefs

focus cache JSON
  |
  +-- topics
  |
  +-- question_candidates

webex_sync_state
  |
  v
Webex cursor / polling mode
```

## 3. Ingestion DAG

```text
Webex tracked room/direct
        |
        v
fetch latest/recent messages
        |
        v
dedupe by message_id
        |
        +-- upsert people
        +-- upsert rooms
        +-- upsert messages
        |
        v
if text not empty
        |
        v
upsert belief_evidence(source=webex_message)
```

```text
Live transcript submit
        |
        +-- upsert people
        +-- upsert rooms
        +-- upsert messages(meetinging-transcript:...)
        |
        x-- no belief_evidence write today
```

## 4. Focus DAG

```text
messages
  |
  v
detailLines
  |
  v
FocusMessageLineParser
  |
  v
FocusNormalizedEvent
  |
  v
FocusClusterSeed
  |
  +-- person key = room + topic words
  |
  +-- space key = semantic tokens
          |
          v
     synonym map + similarity merge
  |
  v
native focus cache
  |
  v
UI detail views
```

## 5. Belief DAG

```text
belief target
    |
    v
scope + entity_key
    |
    +-- space  -> evidence where room_id = entity_key
    |
    +-- person -> evidence where person_id = entity_key
    |             or direct room evidence if room exists
    |
    +-- global -> all evidence
    |
    v
current beliefs + manual beliefs + evidence
    |
    v
hash input
    |
    +-- unchanged and fresh -> skip
    |
    +-- changed/stale/forced -> run
                              |
                              v
                    evidence windows
                    60d -> 90d max
                              |
                              v
                    chunks of 25 evidence rows
                              |
                              v
                    Codex belief JSON
                              |
                              v
                    merge add/update/weaken
                              |
                              v
                    upsert beliefs
```

## 6. Question DAG

```text
space focus cache      person focus cache
       |                       |
       +-----------+-----------+
                   |
                   v
          extract message-like lines
                   |
                   v
       WebexQuestionGeneratorCore
                   |
                   v
          deterministic seed questions
                   |
                   v
             Codex synthesis
                   |
                   v
        question_candidates table
```

Important branch:

```text
no Codex synthesizer
        |
        v
do not publish deterministic seeds
        |
        v
keep existing questions
```

## 7. What Is Missing

```text
Current:

messages ---> evidence ---> beliefs
    |
    +----> focus JSON ---> questions

Missing:

entities
  |
  +-- typed edges
  |
  +-- normalized provenance
  |
  +-- permissions
  |
  +-- retrieval index
  |
  +-- actions/outcomes
```

## 8. Target Architecture

```text
sources
  |
  v
events / documents / fragments
  |
  +--------------------+
  |                    |
  v                    v
entities <---------> edges
  |                    |
  v                    v
claims/beliefs <--> claim_evidence
  |
  v
questions
  |
  v
actions
  |
  v
outcomes
```

## 9. Upgrade Roadmap

```text
P0
messages + transcripts
        |
        v
complete belief_evidence

P1
belief/question JSON evidence
        |
        v
join tables

P2
rooms/people/messages/topics
        |
        v
typed knowledge_edges

P3
focus cache only
        |
        v
persist clusters or remove table

P4
topics table
        |
        v
read it in UI / Ask / Questions

P5
plain SQLite scans
        |
        v
FTS5, then embeddings

P6
questions
        |
        v
actions + outcomes
```


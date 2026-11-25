# Normative Specification Methodology

**Document ID:** NSM-001  
**Version:** 1.0.0  
**Status:** Methodology baseline  
**Purpose:** A repeatable method for producing independent, precise, source-traceable, and testable normative specifications for Accountant Engine and related software domains.

---

## 1. Purpose

This methodology defines how to research, extract, reconcile, draft, review, and approve a normative specification using authoritative sources and explicit project decisions.

It is a drafting and engineering methodology. It is **not itself a domain specification**, does not define Accountant Engine behavior, and does not establish that any implementation conforms to a resulting specification.

## 2. Governing principles

1. **Independent of implementation:** Draft what the system is required to do, not what existing code happens to do.
2. **Authority before convenience:** Prefer authoritative primary sources over summaries, blogs, or implementation examples.
3. **Provenance for every rule:** Each sourced rule must have traceable source evidence. Project decisions must be identified as project decisions.
4. **No silent resolution:** Conflicts, ambiguity, missing authority, or scope disputes must be logged and resolved explicitly.
5. **Testable normative statements:** Requirements must be specific enough to assess conformance objectively.
6. **Normative/informative separation:** Requirements are distinct from rationale, examples, guidance, history, and evidence.
7. **Scope discipline:** Rules belonging to adjacent domains, adapters, hosts, or accounting policy must be excluded or assigned to the proper specification and boundary.
8. **Version and temporal awareness:** Record source edition, effective date, publication date, retrieval/review date, and applicable jurisdiction or context when relevant.
9. **Conformance is separate:** Specification completeness does not prove implementation conformance; conformance needs separate inspection and evidence.
10. **Change control:** A normative rule changes only through a documented review and approval process.

## 3. Terms and classifications

### 3.1 Normative requirement

A binding rule that a conforming implementation, system, process, or actor is required to satisfy within the stated scope.

### 3.2 Informative material

Explanation, rationale, examples, notes, diagrams, background, or commentary that helps readers but does not independently impose requirements.

### 3.3 Source-derived requirement

A requirement whose substance is explicitly imposed or supported by a cited authoritative source. Record the exact source passage/section and its authority and applicability.

### 3.4 Project-defined requirement

A rule adopted by the project where a source does not mandate the particular choice. It may be informed by standards or best practice, but must not be attributed to those sources as a mandatory rule.

### 3.5 Boundary requirement

A rule that defines responsibility, assumptions, or required behavior at a domain or component boundary. Specify which side owns it and avoid duplicating another domain's rule.

### 3.6 Open issue

An unresolved ambiguity, source conflict, missing decision, or question that prevents safe finalization of a rule.

## 4. Source authority and selection

### 4.1 Source-selection order

Use the following priority as a default, adjusting for domain, legal jurisdiction, and scope:

1. Applicable legislation, regulation, or binding legal authority (where relevant).
2. Official standards or normative specifications from the responsible standards body, including the applicable edition and amendments.
3. Official specifications, protocols, API contracts, or policy documents published by the responsible organization or authority.
4. Official registries, datasets, maintenance-agency notices, and authoritative technical publications.
5. Peer-reviewed research and recognized technical publications, when primary normative authority is absent or insufficient.
6. Expert commentary, textbooks, technical articles, and community material for explanation or discovery only—not as authority where a primary source is available.
7. Existing project documentation, code, tests, issue history, and design notes as evidence of current behavior or project decisions—not as proof of what the normative behavior ought to be.

This is a default hierarchy, not an automatic rule for every domain. Applicability and jurisdiction can make a lower-listed source more directly authoritative for a specific subject.

### 4.2 Primary-source rule

For each material rule, seek the original source. Secondary material may help locate or interpret a source but should not replace it in the reference record when the primary source is accessible.

### 4.3 Source register

Create a source register before or during extraction with at least:

| Field | Required information |
| --- | --- |
| Source ID | Stable identifier, e.g. REF-001 |
| Full title | Exact title |
| Publisher / authority | Issuing or maintaining body |
| Edition / version | Version, edition, amendment, or snapshot |
| Publication / effective date | When issued or applicable, if known |
| Retrieval / review date | When checked |
| Official URL | Primary link; archived snapshot if needed |
| Authority / applicability | Why it is authoritative and applicable |
| Scope used | Which topic(s) it supports |
| Limitations | What it does not establish; interpretive caveats |

### 4.4 Source currency

Check whether sources are current, superseded, amended, withdrawn, or subject to effective-date rules. Do not assume that the newest webpage equals the applicable normative edition. For evolving registries, define how updates are controlled and versioned.

## 5. Scope definition before rule extraction

Write a scope statement that identifies:

- Domain and concepts included.
- Behaviors and operations included.
- Explicit exclusions.
- External dependencies and assumptions.
- Actors/components affected.
- Relevant temporal, jurisdictional, or data-version boundaries.
- Adjacent domains and interfaces.

Create a boundary map. Assign each rule to exactly one authoritative specification where possible. Other documents should reference it rather than restate it in competing normative language.

## 6. Research and extraction workflow

### Step 1 — Define the question and scope

State the domain, expected outcomes, exclusions, and questions the specification must answer. Avoid starting with the code's structure or current behavior.

### Step 2 — Search and identify authoritative sources

Search for official standards, specifications, legal sources, registries, and maintenance authorities. Record candidate sources and assess authority, scope, version, and currency.

### Step 3 — Read source sections in context

Do not extract isolated sentences without reviewing definitions, exceptions, scope, normative references, annex status, and relevant surrounding clauses.

### Step 4 — Build a source-extraction ledger

For each potentially relevant source passage, capture:

- Source ID and precise clause/section/page/paragraph locator.
- Faithful short excerpt or careful paraphrase.
- Requirement or concept extracted.
- Applicability to the target domain.
- Whether it is mandatory, recommended, permissive, descriptive, or explanatory in the source.
- Candidate rule ID(s).
- Notes, exceptions, uncertainty, and potential conflicts.

### Step 5 — Classify each candidate rule

Use one of the following classifications:

- **SOURCE-MANDATED:** Direct requirement from an applicable authoritative source.
- **SOURCE-RECOMMENDED:** Source recommendation; decide whether the project adopts it as binding.
- **SOURCE-PERMITTED:** Source allows options; project must select an option if deterministic behavior is required.
- **PROJECT-DECISION:** A deliberate policy choice not mandated by cited authority.
- **DERIVED-REQUIREMENT:** A logically necessary rule derived from source requirements or project choices; document the derivation.
- **BOUNDARY/ASSUMPTION:** Interface obligation or explicit assumption.
- **INFORMATIVE:** Context only; not a conformance requirement.

Do not imply SOURCE-MANDATED status for a PROJECT-DECISION.

### Step 6 — Resolve conflicts and gaps

Maintain a decision/open-issues log. For every conflict:

1. Identify the conflicting passages and sources precisely.
2. Determine whether they apply to the same scope, context, version, and subject.
3. Check authority, precedence, and effective dates.
4. Identify whether the apparent conflict is actually a difference in terminology, context, or optionality.
5. Do not silently choose one interpretation.
6. Record the resolution, rationale, approver, and affected rule IDs—or leave the issue open and mark the specification draft/incomplete.

For source gaps, choose and record an explicit project policy or leave the behavior undefined only when that is an intentional and safe specification choice. Avoid accidental undefined behavior.

### Step 7 — Create the rule inventory

Create stable rule IDs (e.g. DOMAIN-001). Each rule should have a concise title, classification, requirement text, applicability, source trace, rationale where needed, and verification idea. IDs should survive editorial changes; retire IDs rather than reusing them.

### Step 8 — Draft normative statements

Draft requirements using one obligation per statement whenever practical. State the subject, condition, required/prohibited/permitted behavior, and observable outcome. Define terms before relying on them.

### Step 9 — Add behavior and edge cases

For each operation or concept, inspect normal cases, boundaries, invalid inputs, exceptional conditions, error behavior, state transitions, ordering constraints, idempotency/retry implications, and relevant concurrency/atomicity requirements. Include only those aspects within scope.

### Step 10 — Define conformance

Specify what must be true to claim conformance, what parts are mandatory, how optional capabilities are handled, and how behavior can be objectively assessed. Avoid vague claims such as “works correctly” or “is robust.”

### Step 11 — Review and approve

Conduct separate reviews for source accuracy, domain correctness, normative clarity, scope/boundaries, consistency, and testability. Track comments and decisions. Approval status and version must be explicit.

## 7. Normative language

Use one defined requirement vocabulary consistently. If adopting RFC-style keywords, cite the vocabulary source and define its interpretation in the specification or governing policy.

Recommended RFC-style meanings:

- **MUST / MUST NOT:** Absolute requirement or prohibition for conformance.
- **SHOULD / SHOULD NOT:** Strong recommendation; deviations require a documented reason and must be assessed against the specification's intent.
- **MAY:** Permission or optional behavior.

Use uppercase keywords only when they carry the defined normative meaning. Avoid ambiguous substitutes such as “normally,” “ideally,” “as appropriate,” “reasonable,” “properly,” or “should be handled” unless their criteria are defined.

Do not use normative keywords in informative examples, notes, or explanatory passages as though those passages were requirements. Clearly label informative material.

**Important:** RFC 2119/8174 keywords are not automatically appropriate for every internal specification. Choose and document a consistent vocabulary suited to the document and its audience.

## 8. Rule-writing quality criteria

A normative rule should be:

- **Atomic:** Prefer one obligation per rule.
- **Unambiguous:** Terms and conditions have a single reasonable interpretation.
- **Complete:** Required inputs, conditions, outputs, side effects, and failure behavior are specified where relevant.
- **Observable:** Conformance can be assessed from externally observable behavior, state, or defined evidence.
- **Deterministic:** Same defined inputs and conditions produce defined behavior unless nondeterminism is expressly allowed.
- **Scoped:** The rule states or inherits a clear scope and does not invade another domain.
- **Traceable:** Source, project decision, or derivation is recorded.
- **Consistent:** No contradiction with definitions, other rules, interfaces, or cross-domain contracts.
- **Testable:** It supports one or more objective verification approaches.

## 9. Recommended specification structure

Adapt this outline to the domain; omit irrelevant sections and add domain-specific ones:

1. Document status, identifier, version, approval, and change history.
2. Purpose.
3. Scope and exclusions.
4. Normative language and conformance model.
5. Normative references (only references that are genuinely normative).
6. Terms and definitions.
7. Domain model and invariants.
8. Normative rules grouped by topic, with stable IDs.
9. Operations/behavior and state transitions.
10. Input validation and constraints.
11. Precision, limits, ordering, or temporal rules where applicable.
12. Errors and exceptional behavior.
13. Serialization, persistence, or interoperability requirements where in scope.
14. Boundary and integration requirements.
15. Conformance requirements and behavioral acceptance criteria.
16. Informative rationale, examples, diagrams, or appendices—clearly separated.
17. References/source traceability register.
18. Change history.

Do not mechanically include every heading. Structure must fit the scope and improve clarity.

## 10. Traceability model

Maintain traceability in both directions:

**Source → Extracted statement → Classification/decision → Normative rule ID → Acceptance criterion → Verification method**

And reverse:

**Rule ID → Source(s) or project decision → Reason/derivation → Testable criterion**

Suggested traceability table:

| Rule ID | Requirement summary | Classification | Source / clause or decision | Acceptance criterion | Verification |
|---|---|---|---|---|---|
| DOMAIN-001 | … | SOURCE-MANDATED / PROJECT-DECISION | REF-001 §… / DEC-001 | … | Unit / property / contract / review |

A source may support several rules; a rule may depend on several sources. Traceability must preserve these links without copying large copyrighted source passages.

## 11. Decision log and project-policy handling

For project-defined rules, record:

- Decision ID and affected rule ID(s).
- Decision statement.
- Alternatives considered.
- Why sources do not fully determine the behavior (or why the project chooses among permitted options).
- Technical, accounting, security, compatibility, or operational rationale as applicable.
- Approval/owner and date.
- Consequences and revision triggers.

Project decisions may be normative within the project's specification after approval, but they are not thereby requirements of an external standard.

## 12. Conformance and verification planning

For every normative rule, identify an objective criterion. Possible methods include:

- Unit tests.
- Boundary and negative tests.
- Property-based tests.
- Fuzz tests.
- Contract/interface tests.
- Integration tests.
- Static analysis or architecture checks.
- Manual review where behavior cannot be automated.
- Reproducible evidence tied to a commit/build and environment.

The specification defines expected behavior; tests and evidence assess an implementation against it. Do not include implementation status, test pass claims, coverage results, or source-code findings as normative rules.

## 13. Independence from implementation

During normative drafting:

- Do not use current implementation behavior as the default authority.
- Do not copy function names, file paths, types, package layout, or internal algorithms unless the specification intentionally standardizes that interface or algorithm.
- Do not weaken or strengthen a requirement merely to match the current code.
- Do not label rules “implemented,” “verified,” “passing,” or “confirmed” inside the normative rule body.
- If implementation inspection is needed to discover undocumented legacy behavior, place findings in a separate discovery/assessment record. Evaluate whether the behavior should be adopted as a project decision; do not silently promote it into the specification.

## 14. Normative completeness review checklist

### Scope and structure

- [ ] Scope, exclusions, assumptions, and boundaries are explicit.
- [ ] Terms are defined and used consistently.
- [ ] Rule ownership is clear; duplicate normative rules are avoided.

### Source quality

- [ ] Primary authoritative sources were sought and used where available.
- [ ] Source editions, dates, applicability, and official URLs are recorded.
- [ ] Source passages were read in context, including exceptions and definitions.
- [ ] Each source-derived rule has precise traceability.
- [ ] Project policy is not falsely attributed to an external authority.

### Drafting and logic

- [ ] Normative and informative text are clearly distinguished.
- [ ] Requirement vocabulary is defined and used consistently.
- [ ] Rules are atomic, precise, scoped, and objectively testable.
- [ ] No unresolved contradictions or silently resolved conflicts remain.
- [ ] Inputs, outputs, invariants, boundaries, errors, and exceptional behavior are specified where applicable.
- [ ] Optionality and allowed implementation variation are explicit.

### Conformance

- [ ] Conformance criteria are objectively stated.
- [ ] Every rule has an acceptance criterion or verification concept.
- [ ] The specification does not claim implementation conformance without separate evidence.

### Governance

- [ ] Version, status, owner/approver, and change process are recorded.
- [ ] Open issues are resolved or clearly prevent approval.
- [ ] Change history identifies normative changes.

## 15. Specification maturity states

Use status labels to describe document maturity, not implementation:

- **Research Draft:** Sources and scope still being investigated.
- **Normative Draft:** Rules are drafted but review/open issues remain.
- **Review Candidate:** Source trace, completeness, and consistency review are substantially complete; awaiting approval.
- **Approved Normative Specification:** Approved reference baseline for conformance assessment.
- **Superseded:** Replaced by a later approved version; retained for history.

Implementation conformance should be reported separately (e.g. Not Assessed, Non-Conformant, Partially Conformant, Conformant, or assessment blocked), with evidence and commit/build references. Define the exact status scheme in the assessment process.

## 16. Change control

Any proposed normative change must:

1. Identify affected rule IDs and specifications.
2. Identify whether the change is source-driven, a project-policy change, a correction, or clarification.
3. Review source updates and applicability.
4. Assess compatibility, boundary effects, and downstream rules/tests.
5. Record rationale, approval, and version change.
6. Update traceability and acceptance criteria.
7. Preserve retired rule IDs and change history.

Editorial-only changes that do not alter requirement meaning should be identified as such.

## 17. Official methodology references

These sources inform how specifications are structured, drafted, and reviewed. They are **methodology references**, not automatically domain authorities for Accountant Engine rules.

1. **ISO/IEC Directives, Part 2 — Principles and rules for the structure and drafting of ISO and IEC documents.** Primary reference for drafting structure, clarity, verbal forms, and normative/informative conventions.  
   <https://www.iso.org/directives-and-policies.html>
2. **ISO Drafting Standards resources.** Official templates, drafting guidance, ISO House Style, and related resources.  
   <https://www.iso.org/drafting-standards.html>
3. **ISO House Style.** Editorial language and presentation guidance; points back to ISO/IEC Directives Part 2 as the primary drafting reference.  
   <https://www.iso.org/ISO-house-style.html>
4. **W3C Manual of Style — Normative material.** Guidance for distinguishing normative/informative sections and using RFC-style keywords.  
   <https://www.w3.org/guide/manual-of-style/>
5. **RFC 2119 — Key words for use in RFCs to Indicate Requirement Levels.**  
   <https://www.rfc-editor.org/rfc/rfc2119>
6. **RFC 8174 — Ambiguity of Uppercase vs Lowercase in RFC 2119 Key Words.**  
   <https://www.rfc-editor.org/rfc/rfc8174>
7. **W3C QA Framework: Specification Guidelines.** Specification clarity, conformance, and allowing variation among conforming implementations.  
   <https://www.w3.org/TR/qaframe-spec/>
8. **NIST — Writing Quality Specifications.** Guidance focused on clarity, avoiding ambiguity, conformance, and permitted variation.  
   <https://www.nist.gov/publications/writing-quality-specifications>

Check current editions and official pages when applying this methodology to a new specification. This document does not reproduce copyrighted standards text; it summarizes a process and links to official references.

## 18. Operational workflow summary

```text
Scope and boundary definition
        ↓
Source discovery and authority assessment
        ↓
Source register + source extraction ledger
        ↓
Classification (source mandate / recommendation / permission / project decision / derived / informative)
        ↓
Conflict, gap, and decision log
        ↓
Stable rule inventory + traceability
        ↓
Normative drafting + edge/error/boundary analysis
        ↓
Conformance criteria and verification planning
        ↓
Independent review (source / domain / clarity / consistency / testability)
        ↓
Approval and versioned normative baseline
        ↓
Separate implementation conformance assessment and evidence
```

---

## 19. Non-negotiable rule for Accountant Engine specifications

**A specification is the approved definition of required behavior. The code is an implementation to be assessed against that definition—not the authority used to decide what the definition should say.**

If a source does not mandate a detail, either adopt it through an explicit, traceable project decision or leave it deliberately unspecified with the implications clearly stated. Never conceal project policy as an external standard requirement.

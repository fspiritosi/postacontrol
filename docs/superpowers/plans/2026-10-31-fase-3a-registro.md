# SDD ledger — plan: docs/superpowers/plans/2026-10-31-fase-3a-redactor-rico.md

Branch fase-3a from main 83388a9; plan commit ea2eb00. Spec: docs/superpowers/specs/2026-10-30-postacontrol-design.md (reachable).

## Pre-flight scan

| Pair / Task | Produces vs consumes | Finding |
|---|---|---|
| T1→T2 | EmailHTML, ExtractInlineImages, htmlx.InlineImage → T2 maps to mimex.OutInline in deliver | consistent |
| T1 vs existing compose.go | T1 says TightParagraphs "queda como helper interno o se borra"; compose.go:248 still calls htmlx.TightParagraphs until T2 | conflict → P1 |
| T2↔T3 | both edit compose.go deliver + compose_test.go; T3 extends deliver after T2's inline handling | consistent (sequential) |
| T3→T4 | StageFiles(ctx, []string) ([]StagedAttachment, error) matches handleDrop's stage func type; LoadImage, StagedInfo signatures | consistent |
| T3 formatSize ↔ T6/T7 formatSize | Go must mirror frontend lib/format.ts formatSize ("25.0 MB", "6.0 MB") | consistent |
| T4→T6/T7 | bindings PickImage/PickAttachments/StagedInfo; event "files-staged" {attachments,error}; id "compose-drop" | consistent |
| T5→T6 | onPickImage prop, "Imagen" item hidden without prop; T5 installs extension-image/file-handler deps used by T6 | consistent |
| T6↔T7 | both edit Composer.tsx / Composer.test.tsx | consistent (sequential) |
| T1 self | tests vs code | consistent |
| T2 self | tests vs MIME structure | consistent |
| T3 self | files list includes core.go for prune; DeleteLocalDraft deletes staged | consistent |
| T4 self | handleDrop pure fn tested; main.go change | consistent |
| T5 self | test "y/o no abre el menú" vs allowedPrefixes default | consistent |
| T6 self | consistent |
| T7 self | consistent |
| T8 | manual acceptance by Fabricio | → P2 |

## Rulings
- P1: Ruling: T1 keeps htmlx.TightParagraphs exported (EmailHTML reuses it); T2 switches compose.go to EmailHTML and may then unexport/delete it with its test migrated — avoids a broken build between T1 and T2 — cost if wrong: one dead exported helper for one commit.
- P2: Ruling: Task 8 is Fabricio's manual acceptance; not dispatched — spec says real-account tests are manual — cost if wrong: none.

## Progress
Task 1: complete (commits ea2eb00..51d8cea, review clean)
Task 1: minor (deferred): dedup by bytes keeps first MIME when same bytes declared with different types (email.go:162)
Task 1: minor (deferred): bytes.TrimSpace(data) empty check vs len(data)==0 (email.go:192)
Task 1: minor (deferred): <ol start=N> ignored in ToText
Task 1: minor (deferred): srcset data: URIs not inspected by ExtractInlineImages (TipTap never emits srcset)
Task 1: note: no separate RED run captured (tests written alongside code)
Task 2: complete (commits 51d8cea..469ffb9, review clean)
Task 2: minor (deferred): writePart iterates extra-header map → nondeterministic header order (mimex/build.go)
Task 2: minor (deferred): htmlx.TightParagraphs exported but unused in production
Task 2: minor (deferred): TestSend_PastedImageTravelsAsCID asserts raw "Content-Id:" casing
Task 3: review 1 — Important: prune disabled by "attachments":null (NOT IN NULL) → fix round 1 dispatched (resume implementer)
Task 3: minor (deferred): Go formatSize %.1f rounds half-to-even vs JS toFixed tie-up (1310720 B → 1.2 vs 1.3 MB)
Task 3: minor (deferred): original attachments counted by IMAP BODYSTRUCTURE (encoded ~1.37x) size vs decoded staged/inline
Task 3: minor (deferred): DeleteLocalDraft scans ListLocalDrafts and deletes staged before the row (should delete row first / one tx)
Task 3: minor (deferred): store.DeleteLocalDraft has no status guard while sending (UI-only guard)
Task 3: minor (deferred): prune at open uses time.Now not c.now
- T3-R1: Ruling: Task 7's UI size total uses the same `size` fields the core uses (StagedAttachment.size, Attachment.size) so UI and enqueue agree — mixed encoded/decoded basis accepted for now — cost if wrong: forwards near 25 MB rejected ~37% early.
Task 3: fix round 1/5 (1 addressed, 0 open; commits b68e3dc..27b4deb)
Task 3: complete (commits 469ffb9..27b4deb, review clean)
- T4-R1: Ruling: SetupFileDrop(app, win, svc *MailService) instead of (win, core, emit) — core is created in ServiceStartup after main wires the window — cost if wrong: none (handleDrop contract unchanged).
Task 4: complete (commits 27b4deb..91def79, review clean)
Task 4: minor (deferred): svc.core written in ServiceStartup, read in drop goroutine without sync (theoretical race)
Task 4: minor (deferred): UserError with empty Msg in handleDrop → Error "" looks like success
Task 4: note: ⚠️ frontend must put data-file-drop-target AND id="compose-drop" on the composer (Task 7 brief already says both)
Task 5: review 1 — Critical: popup under Radix Dialog; Important: Esc closes composer, "/"→Enlace inserts nothing (spec), stale link popover state, allowSpaces replaces text ("A / B"+Enter → table) → fix round 1 dispatched (resume implementer)
Task 5: minor (deferred): bubble aria-pressed can go stale (useEditor doesn't re-render on selection; use useEditorState)
Task 5: minor (deferred): no test that the popup is removed when editor unmounts with menu open
Task 5: fix round 1/5 (5 addressed, 0 open; commits f4ea3ac..0829442)
Task 5: complete (commits 91def79..0829442, review clean)
Task 5: minor (deferred): "/zzz " trailing space briefly shows "Sin resultados" until next char
Task 5: minor (deferred): popup placement inside transformed DialogContent verified only by reading floating-ui (manual check in T8)
Task 6: complete (commits 0829442..60ef644, review clean)
Task 6: ⚠️ resolved: Go send path drops non png/jpeg/gif/webp data images (ExtractInlineImages, T1) and enforces 25 MB total (T3); outbox keeps data: URIs in bodyHtml
Task 6: minor (deferred): FileHandler allowedMimeTypes pre-filter → unsupported pasted image types dropped silently (imageError type branch dead on paste)
Task 6: minor (deferred): multiple pasted images insert in FileReader completion order
Task 6: minor (deferred): pasted HTML data:image bypasses 5 MB per-image check (only 25 MB total enforced); selector admits svg/bmp (dropped at send)
Task 6: minor (deferred): async insert after composer closed relies on catch; guard editor.isDestroyed
Task 6: minor (deferred): paste with files + text/html also pastes HTML (consumePasteEvent not set)
Task 6: minor (deferred): no jsdom test for real File paste
Task 7: review 1 — Important: reopening a forward from outbox hides original attachments (no chips, not removable, not counted in guard) → fix round 1 dispatched (resume implementer)
Task 7: minor (deferred): pickAttachments result after composer closed lands in unmounted state (core prune sweeps)
Task 7: minor (deferred): a later successful drop doesn't clear an earlier drop error
Task 7: minor (deferred): no test for listener cleanup on close/reopen, drop while sending, guard with kept vs skipped originals
Task 7: fix round 1/5 (1 addressed, 0 open; commits 92c3a41..fe776e7)
Task 7: complete (commits 60ef644..fe776e7, review clean)
Task 7: minor (deferred): deleted original → PrepareCompose fails → no chips but Go still tries originals (send fails as before)
Final review (opus, 83388a9..fe776e7): With fixes — Important: (1) ToText garbles TipTap <li><p>/<td><p> shapes; (2) paste with files+text/html inserts extra picture (upgraded T6 minor). Minors 3-10.
- FR-R1: Ruling: fix wave covers Important 1-2 + Minors 3 (quote images in checkSize), 4 (store-only lookup in checkSize), 5 (lazy getHTML), 7 (limit wording), 9 (batch-size check in StageFiles), 10 (reopened reply/forward shows quote); defer 6 (Outlook width attr) and 8 (prune on discard / periodic) to 3B — cheap fixes that affect acceptance go now; the two deferred need new decoding/bindings — cost if wrong: large screenshots full-size in Outlook desktop; orphaned staged blobs until restart.
Final fix wave: commits fe776e7..1115218; re-review (opus): all 8 findings ADDRESSED, no new Critical/Important.
Final: parked — totext glues inline text before first <p> in li/td ("- textomás") — Ruling: TipTap never emits it; only affects quoted received mail's plain text; real, deferred — cost if wrong: occasional glued words in plain-text quotes.
Final: parked — equal-rounding wording remains in batch-sum, too_large and the UI "Los adjuntos suman" messages; UI/Go texts differ ("adjuntos" vs "archivos") — Ruling: cosmetic, deferred — cost if wrong: a confusing "25.0 MB de 25.0 MB" message right at the limit.
Final: parked — nested lists flatten in ToText (pre-existing); paste with HTML containing <img> + file may duplicate image (as prescribed) — Ruling: deferred — cost if wrong: duplicate image on some browser copies.

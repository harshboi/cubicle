const state = {
  user: null,
  notes: [],
  currentNote: null,
  recording: null,
  lastSavedNoteId: null,
  captureSource: "microphone",
  selectedNoteIds: new Set(),
  showOriginalTranscript: false,
};

const routes = {
  "/": "home",
  "/record": "record",
  "/settings": "settings",
};

document.addEventListener("DOMContentLoaded", async () => {
  await bootstrap();
});

async function bootstrap() {
  const me = await api("/api/me");
  state.user = me.user;
  renderProfile();
  bindNavigation();
  bindActions();
  await refreshNotes();
  routeTo(window.location.pathname, false);
}

function bindNavigation() {
  document.querySelectorAll("[data-route]").forEach((link) => {
    link.addEventListener("click", (event) => {
      event.preventDefault();
      routeTo(new URL(link.href).pathname, true);
    });
  });
  window.addEventListener("popstate", () => routeTo(window.location.pathname, false));
}

function bindActions() {
  document.getElementById("record-top-button").addEventListener("click", () => routeTo("/record", true));
  document.getElementById("logout-button").addEventListener("click", logout);
  document.getElementById("start-button").addEventListener("click", startRecording);
  document.getElementById("stop-button").addEventListener("click", stopRecording);
  document.getElementById("search-input").addEventListener("input", renderNotes);
  document.getElementById("copy-note-button").addEventListener("click", copyCurrentNote);
  document.getElementById("download-note-button").addEventListener("click", downloadCurrentNote);
  document.getElementById("delete-note-button").addEventListener("click", deleteCurrentNote);
  document.getElementById("toggle-original-button").addEventListener("click", toggleOriginalTranscript);
  document.getElementById("select-all-notes").addEventListener("change", toggleAllVisibleNotes);
  document.getElementById("bulk-delete-button").addEventListener("click", deleteSelectedNotes);
  document.getElementById("open-saved-note-button").addEventListener("click", openLastSavedNote);
  document.getElementById("new-recording-button").addEventListener("click", resetRecordView);
  document.getElementById("note-title-input").addEventListener("change", saveCurrentNoteTitle);
  document.querySelectorAll("[data-capture-source]").forEach((button) => {
    button.addEventListener("click", () => selectCaptureSource(button.dataset.captureSource));
  });
  document.getElementById("diarization-toggle").addEventListener("change", () => {
    document.getElementById("diarization-toggle").checked = false;
    document.getElementById("diarization-label").textContent = "disabled";
  });
}

async function api(path, options = {}) {
  const response = await fetch(path, {
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
    ...options,
  });
  if (response.status === 401) {
    window.location.href = "/login";
    return null;
  }
  if (!response.ok) {
    throw new Error(`Request failed: ${response.status}`);
  }
  return response.json();
}

async function logout() {
  const response = await fetch("/api/logout", { method: "POST", credentials: "same-origin" });
  const payload = response.ok ? await response.json() : null;
  window.location.href = payload?.logout_url || "/logout";
}

function routeTo(path, push) {
  if (path.startsWith("/notes/")) {
    showView("note");
    loadNote(path.split("/").pop());
  } else {
    showView(routes[path] || "home");
  }
  if (push) history.pushState({}, "", path);
}

function showView(name) {
  document.querySelectorAll(".view").forEach((view) => view.classList.remove("active-view"));
  document.getElementById(`${name}-view`).classList.add("active-view");
  document.querySelectorAll("[data-route]").forEach((link) => {
    link.classList.toggle("active", link.dataset.route === name || (name === "home" && link.dataset.route === "home"));
  });
}

function renderProfile() {
  document.getElementById("profile-name").textContent = state.user.display_name;
  document.getElementById("profile-email").textContent = state.user.email;
  document.getElementById("profile-initial").textContent = state.user.display_name.trim().charAt(0).toUpperCase();
  document.getElementById("settings-email").textContent = state.user.email;
}

async function refreshNotes() {
  const payload = await api("/api/notes");
  state.notes = (payload.notes || []).sort((left, right) => new Date(right.created_at) - new Date(left.created_at));
  pruneSelectedNotes();
  renderNotes();
}

function renderNotes() {
  const query = document.getElementById("search-input").value.trim().toLowerCase();
  const notes = state.notes.filter((note) => {
    const displayTitle = formatNoteListTitle(note).toLowerCase();
    const transcriptTitle = String(note.title || "").toLowerCase();
    return !query || displayTitle.includes(query) || transcriptTitle.includes(query);
  });
  const list = document.getElementById("notes-list");
  list.innerHTML = "";
  document.getElementById("empty-notes").hidden = notes.length > 0;
  document.getElementById("bulk-actions").hidden = state.notes.length === 0;
  document.getElementById("recordings-count").textContent = notes.length
    ? `${notes.length} recording${notes.length === 1 ? "" : "s"}`
    : "No recordings yet";
  notes.forEach((note) => {
    const displayTitle = formatNoteListTitle(note);
    const selected = state.selectedNoteIds.has(note.note_id);
    const card = document.createElement("div");
    card.className = "note-card";
    card.classList.toggle("selected", selected);
    card.dataset.noteId = note.note_id;
    card.innerHTML = `
      <label class="note-select">
        <input type="checkbox" class="note-checkbox" ${selected ? "checked" : ""} aria-label="Select ${escapeHtml(displayTitle)}">
      </label>
      <button type="button" class="note-open-button">
        <span class="note-title-row">
          <span class="note-title">${escapeHtml(displayTitle)}</span>
          <span class="note-status">${escapeHtml(note.status || "saved")}</span>
        </span>
        <span class="note-meta">${formatDate(note.created_at)} - ${formatDuration(note.duration_ms)} - ${note.word_count || 0} words</span>
      </button>
    `;
    card.querySelector(".note-checkbox").addEventListener("change", (event) => {
      setNoteSelected(note.note_id, event.target.checked);
      card.classList.toggle("selected", event.target.checked);
    });
    card.querySelector(".note-open-button").addEventListener("click", () => routeTo(`/notes/${note.note_id}`, true));
    list.appendChild(card);
  });
  updateBulkActions();
}

async function loadNote(noteId) {
  const payload = await api(`/api/notes/${noteId}`);
  state.currentNote = payload;
  state.showOriginalTranscript = false;
  document.getElementById("note-title-input").value = payload.note.title;
  renderNoteTranscript(payload.transcript);
  renderNoteIntelligence(payload.note, payload.transcript);
}

function renderNoteTranscript(transcript) {
  const article = document.getElementById("note-transcript");
  article.innerHTML = "";
  const segments = transcript?.segments || [];
  const hasOriginal = segments.some((segment) => hasSourceVariant(segment));
  const toggle = document.getElementById("toggle-original-button");
  toggle.hidden = !hasOriginal;
  toggle.textContent = state.showOriginalTranscript ? "Show English" : "Show original";
  if (!segments.length) {
    article.innerHTML = '<div class="empty-state"><h2>No transcript saved</h2></div>';
    return;
  }
  let lastSpeaker = null;
  segments.forEach((segment) => {
    const block = document.createElement("p");
    block.className = "saved-segment";
    if (segment.speaker_id && segment.speaker_id !== lastSpeaker) {
      block.innerHTML = `<strong>${escapeHtml(formatSpeakerLabel(segment.speaker_id))}</strong> ${escapeHtml(displaySegmentText(segment))}`;
      lastSpeaker = segment.speaker_id;
    } else {
      block.textContent = displaySegmentText(segment);
    }
    article.appendChild(block);
  });
}

function displaySegmentText(segment) {
  if (state.showOriginalTranscript) {
    return segment.source_text || segment.text || "";
  }
  return segment.translated_text || segment.text || "";
}

function hasSourceVariant(segment) {
  const source = String(segment.source_text || "").trim();
  const visible = String(segment.translated_text || segment.text || "").trim();
  return Boolean(source && visible && source !== visible);
}

function renderNoteIntelligence(note, transcript) {
  const source = transcript?.text_intelligence || note || {};
  const summary = source.summary || "";
  const actionItems = Array.isArray(source.action_items) ? source.action_items : [];
  const decisions = Array.isArray(source.decisions) ? source.decisions : [];
  const openQuestions = Array.isArray(source.open_questions) ? source.open_questions : [];
  document.getElementById("summary-text").textContent = summary;
  renderTextList("action-items-list", actionItems.map(formatActionItem).filter(Boolean));
  renderTextList("decisions-list", decisions);
  renderTextList("open-questions-list", openQuestions);
  document.getElementById("summary-panel").hidden = !summary;
  document.getElementById("action-items-panel").hidden = actionItems.length === 0;
  document.getElementById("decisions-panel").hidden = decisions.length === 0;
  document.getElementById("open-questions-panel").hidden = openQuestions.length === 0;
  document.getElementById("note-intelligence").hidden = !summary && !actionItems.length && !decisions.length && !openQuestions.length;
}

function renderTextList(elementId, items) {
  const list = document.getElementById(elementId);
  list.innerHTML = "";
  items.forEach((item) => {
    if (!item) return;
    const entry = document.createElement("li");
    entry.textContent = item;
    list.appendChild(entry);
  });
}

function formatActionItem(item) {
  if (typeof item === "string") return item;
  if (!item || typeof item !== "object") return "";
  const parts = [item.task, item.owner ? `Owner: ${item.owner}` : "", item.due_date ? `Due: ${item.due_date}` : ""]
    .filter(Boolean);
  return parts.join(" - ");
}

async function saveCurrentNoteTitle() {
  if (!state.currentNote) return;
  const title = document.getElementById("note-title-input").value;
  const payload = await api(`/api/notes/${state.currentNote.note.note_id}`, {
    method: "PATCH",
    body: JSON.stringify({ title }),
  });
  state.currentNote.note = payload.note;
  await refreshNotes();
}

async function copyCurrentNote() {
  if (!state.currentNote) return;
  const text = (state.currentNote.transcript?.segments || []).map((segment) => displaySegmentText(segment)).join("\n");
  await navigator.clipboard.writeText(text);
}

function toggleOriginalTranscript() {
  if (!state.currentNote) return;
  state.showOriginalTranscript = !state.showOriginalTranscript;
  renderNoteTranscript(state.currentNote.transcript);
}

function downloadCurrentNote() {
  if (!state.currentNote) return;
  let timezone = "";
  try {
    timezone = Intl.DateTimeFormat().resolvedOptions().timeZone || "";
  } catch {
    timezone = "";
  }
  const suffix = timezone ? `?timezone=${encodeURIComponent(timezone)}` : "";
  window.location.href = `/api/notes/${state.currentNote.note.note_id}/download.txt${suffix}`;
}

async function deleteCurrentNote() {
  if (!state.currentNote) return;
  const noteId = state.currentNote.note.note_id;
  await api(`/api/notes/${noteId}`, { method: "DELETE" });
  state.selectedNoteIds.delete(noteId);
  state.currentNote = null;
  await refreshNotes();
  routeTo("/", true);
}

async function deleteSelectedNotes() {
  if (!state.selectedNoteIds.size) return;
  const noteIds = Array.from(state.selectedNoteIds);
  const label = `${noteIds.length} recording${noteIds.length === 1 ? "" : "s"}`;
  if (!window.confirm(`Delete ${label}?`)) return;
  const button = document.getElementById("bulk-delete-button");
  button.disabled = true;
  button.textContent = "Deleting";
  try {
    await api("/api/notes/bulk-delete", {
      method: "POST",
      body: JSON.stringify({ note_ids: noteIds }),
    });
    state.selectedNoteIds.clear();
    await refreshNotes();
  } finally {
    button.textContent = "Delete selected";
    updateBulkActions();
  }
}

async function startRecording() {
  if (state.recording) return;
  hideSavedTranscriptActions();
  resetTranscript();
  document.getElementById("start-button").disabled = true;
  setCaptureSourceControlsDisabled(true);
  const source = state.captureSource;
  setRecordingStatus("starting", "Starting", source === "tab" ? "Requesting tab audio" : "Requesting microphone");
  let stream;
  try {
    stream = await getRecordingStream(source);
  } catch (error) {
    stream?.getTracks().forEach((track) => track.stop());
    document.getElementById("start-button").disabled = false;
    setCaptureSourceControlsDisabled(false);
    setRecordingStatus("error", "Error", source === "tab" ? "Tab audio unavailable" : "Microphone unavailable");
    return;
  }
  if (!stream.getAudioTracks().length) {
    stream.getTracks().forEach((track) => track.stop());
    document.getElementById("start-button").disabled = false;
    setCaptureSourceControlsDisabled(false);
    setRecordingStatus("error", "Error", "No audio track selected");
    return;
  }
  const socket = new WebSocket(`${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/ws/record`);
  socket.binaryType = "arraybuffer";
  const recorder = await createPcmStreamer(stream, (frame, level) => {
    updateMicLevel(level);
    if (socket.readyState === WebSocket.OPEN) socket.send(frame);
  });
  state.recording = { stream, socket, recorder, recorderStopped: false, segments: new Map(), noteId: null, source };
  socket.addEventListener("open", () => {
    socket.send(JSON.stringify({
      type: "start_recording",
      language_mode: "multilingual_to_english",
      diarization_enabled: false,
      client_timestamp: new Date().toISOString(),
    }));
    recorder.start();
    document.getElementById("start-button").disabled = true;
    document.getElementById("stop-button").disabled = false;
    setRecordingStatus("live", "Live", source === "tab" ? "Recording tab audio" : "Recording microphone");
  });
  socket.addEventListener("message", (event) => handleRecordingEvent(JSON.parse(event.data)));
  socket.addEventListener("close", async () => {
    cleanupRecorder();
    document.getElementById("start-button").disabled = false;
    document.getElementById("stop-button").disabled = true;
    setCaptureSourceControlsDisabled(false);
    updateMicLevel(0);
    await refreshNotes();
  });
}

async function getRecordingStream(source) {
  if (source === "tab") {
    return navigator.mediaDevices.getDisplayMedia({
      video: true,
      audio: { echoCancellation: false, noiseSuppression: false, autoGainControl: false },
    });
  }
  return navigator.mediaDevices.getUserMedia({
    audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true },
  });
}

function stopRecording() {
  if (!state.recording) return;
  setRecordingStatus("saving", "Saving", "Finalizing transcript");
  stopRecorder();
  if (state.recording.socket.readyState === WebSocket.OPEN) {
    state.recording.socket.send(JSON.stringify({ type: "stop_recording" }));
  }
}

function cleanupRecorder() {
  if (!state.recording) return;
  stopRecorder();
  state.recording.stream.getTracks().forEach((track) => track.stop());
  state.recording = null;
}

function stopRecorder() {
  if (!state.recording || state.recording.recorderStopped) return;
  state.recording.recorderStopped = true;
  state.recording.recorder.stop();
}

function handleRecordingEvent(event) {
  if (event.type === "recording_started") {
    document.getElementById("session-id").textContent = event.note_id;
    if (state.recording) state.recording.noteId = event.note_id;
    return;
  }
  if (event.type === "partial_transcript" || event.type === "final_transcript") {
    upsertTranscriptSegment(event);
    return;
  }
  if (event.type === "translated_transcript") {
    applyTranslatedTranscript(event);
    return;
  }
  if (event.type === "speaker_update") {
    applySpeakerUpdate(event);
    return;
  }
  if (event.type === "diarization_status") {
    updateDiarizationStatus(event);
    return;
  }
  if (event.type === "recording_stopped") {
    setRecordingStatus("complete", "Saved", "Transcript saved");
    showSavedTranscriptActions(event.note_id || state.recording?.noteId);
    return;
  }
  if (event.type === "error") {
    setRecordingStatus("error", "Error", event.message || "Recording failed");
  }
}

function resetTranscript() {
  const output = document.getElementById("transcript-output");
  output.innerHTML = "";
}

function upsertTranscriptSegment(event) {
  const segmentId = event.segment_id;
  if (!segmentId) return;
  const previous = state.recording?.segments.get(segmentId) || readRenderedTranscriptSegment(segmentId) || {};
  const incomingText = event.text ?? previous.text ?? "";
  const incomingSourceText = event.source_text ?? previous.source_text ?? incomingText;
  const hasCompletedTranslation = previous.translation_status === "complete" && previous.translated_text;
  const preserveCompletedTranslation =
    hasCompletedTranslation &&
    event.type === "final_transcript" &&
    sameTranscriptText(previous.source_text || "", incomingSourceText || incomingText);
  const next = {
    text: preserveCompletedTranslation ? previous.translated_text : incomingText,
    speaker_id: event.speaker_id ?? previous.speaker_id ?? null,
    is_partial: event.type === "partial_transcript",
    source_text: incomingSourceText,
    translated_text: event.translated_text ?? previous.translated_text ?? null,
    translation_status: event.translation_status ?? previous.translation_status ?? "not_requested",
    translation_model: event.translation_model ?? previous.translation_model ?? null,
  };
  state.recording?.segments.set(segmentId, next);
  renderTranscriptSegment(segmentId, next);
}

function applyTranslatedTranscript(event) {
  const segmentId = event.segment_id;
  if (!segmentId || !event.text) return;
  const previous = state.recording?.segments.get(segmentId) || readRenderedTranscriptSegment(segmentId) || {};
  const next = {
    ...previous,
    text: event.text,
    source_text: event.source_text ?? previous.source_text ?? previous.text ?? "",
    translated_text: event.text,
    translation_status: event.translation_status || "complete",
    translation_model: event.translation_model ?? previous.translation_model ?? null,
    is_partial: false,
  };
  state.recording?.segments.set(segmentId, next);
  renderTranscriptSegment(segmentId, next);
}

function applySpeakerUpdate(event) {
  const segmentId = event.segment_id;
  if (!segmentId || !event.speaker_id) return;
  const previous = state.recording?.segments.get(segmentId) || readRenderedTranscriptSegment(segmentId);
  if (!previous) return;
  const next = { ...previous, speaker_id: event.speaker_id, is_partial: false };
  state.recording?.segments.set(segmentId, next);
  renderTranscriptSegment(segmentId, next);
}

function renderTranscriptSegment(segmentId, data) {
  const output = document.getElementById("transcript-output");
  output.querySelector(".empty-transcript")?.remove();
  let segment = document.getElementById(`segment-${segmentId}`);
  if (!segment) {
    segment = document.createElement("div");
    segment.id = `segment-${segmentId}`;
    segment.className = "transcript-segment";
    output.appendChild(segment);
  }
  segment.classList.toggle("partial", Boolean(data.is_partial));
  segment.dataset.text = data.text || "";
  segment.dataset.speakerId = data.speaker_id || "";
  segment.dataset.partial = data.is_partial ? "true" : "false";
  segment.dataset.sourceText = data.source_text || data.text || "";
  segment.dataset.translatedText = data.translated_text || "";
  segment.dataset.translationStatus = data.translation_status || "";
  segment.dataset.translationModel = data.translation_model || "";
  const speaker = data.speaker_id ? `<span>${escapeHtml(formatSpeakerLabel(data.speaker_id))}</span>` : "";
  segment.innerHTML = `${speaker}<p>${escapeHtml(data.text || "")}</p>`;
}

function readRenderedTranscriptSegment(segmentId) {
  const segment = document.getElementById(`segment-${segmentId}`);
  if (!segment) return null;
  return {
    text: segment.dataset.text || "",
    speaker_id: segment.dataset.speakerId || null,
    is_partial: segment.dataset.partial === "true",
    source_text: segment.dataset.sourceText || "",
    translated_text: segment.dataset.translatedText || null,
    translation_status: segment.dataset.translationStatus || "",
    translation_model: segment.dataset.translationModel || null,
  };
}

function sameTranscriptText(left, right) {
  return normalizeTranscriptText(left) === normalizeTranscriptText(right);
}

function normalizeTranscriptText(value) {
  return String(value || "").replace(/\s+/g, " ").trim();
}

function updateDiarizationStatus(event) {
  document.getElementById("diarization-toggle").checked = false;
  document.getElementById("diarization-label").textContent = "disabled";
}

function showSavedTranscriptActions(noteId) {
  if (!noteId) return;
  state.lastSavedNoteId = noteId;
  document.getElementById("saved-note-actions").hidden = false;
}

function hideSavedTranscriptActions() {
  state.lastSavedNoteId = null;
  document.getElementById("saved-note-actions").hidden = true;
}

function openLastSavedNote() {
  if (!state.lastSavedNoteId) return;
  routeTo(`/notes/${state.lastSavedNoteId}`, true);
}

function resetRecordView() {
  hideSavedTranscriptActions();
  resetTranscript();
  document.getElementById("session-id").textContent = "not started";
  document.getElementById("diarization-toggle").checked = false;
  document.getElementById("diarization-label").textContent = "disabled";
  setRecordingStatus("idle", "Ready", sourceIdleSubtitle());
}

function selectCaptureSource(source) {
  if (state.recording || !["microphone", "tab"].includes(source)) return;
  state.captureSource = source;
  document.querySelectorAll("[data-capture-source]").forEach((button) => {
    button.classList.toggle("active", button.dataset.captureSource === source);
  });
  document.getElementById("input-source-label").textContent = source === "tab" ? "Tab audio" : "Microphone";
  setRecordingStatus("idle", "Ready", sourceIdleSubtitle());
}

function setCaptureSourceControlsDisabled(disabled) {
  document.querySelectorAll("[data-capture-source]").forEach((button) => {
    button.disabled = disabled;
  });
}

function sourceIdleSubtitle() {
  return state.captureSource === "tab" ? "Tab audio idle" : "Microphone idle";
}

async function createPcmStreamer(stream, onFrame) {
  const context = new AudioContext();
  const source = context.createMediaStreamSource(stream);
  const processor = context.createScriptProcessor(4096, 1, 1);
  const silentGain = context.createGain();
  silentGain.gain.value = 0;
  let running = false;
  processor.onaudioprocess = (event) => {
    if (!running) return;
    const input = event.inputBuffer.getChannelData(0);
    const level = rms(input);
    const pcm = downsampleTo16k(input, context.sampleRate);
    if (pcm.byteLength) onFrame(pcm, level);
  };
  source.connect(processor);
  processor.connect(silentGain);
  silentGain.connect(context.destination);
  return {
    start() {
      running = true;
    },
    stop() {
      running = false;
      processor.disconnect();
      source.disconnect();
      silentGain.disconnect();
      context.close();
    },
  };
}

function downsampleTo16k(input, inputRate) {
  const targetRate = 16000;
  if (inputRate === targetRate) return floatToInt16(input).buffer;
  const ratio = inputRate / targetRate;
  const outputLength = Math.floor(input.length / ratio);
  const output = new Float32Array(outputLength);
  for (let i = 0; i < outputLength; i += 1) {
    output[i] = input[Math.floor(i * ratio)];
  }
  return floatToInt16(output).buffer;
}

function floatToInt16(input) {
  const output = new Int16Array(input.length);
  for (let i = 0; i < input.length; i += 1) {
    const sample = Math.max(-1, Math.min(1, input[i]));
    output[i] = sample < 0 ? sample * 0x8000 : sample * 0x7fff;
  }
  return output;
}

function rms(samples) {
  let sum = 0;
  for (let i = 0; i < samples.length; i += 1) sum += samples[i] * samples[i];
  return Math.min(1, Math.sqrt(sum / samples.length) * 4);
}

function setRecordingStatus(status, title, subtitle) {
  document.getElementById("live-title").textContent = title;
  document.getElementById("live-subtitle").textContent = subtitle;
  document.getElementById("live-badge").textContent = status;
  document.getElementById("live-icon").className = `pulse-icon ${status}`;
}

function updateMicLevel(level) {
  document.getElementById("mic-level").textContent = `${Math.round(level * 100)}%`;
}

function formatDate(value) {
  return new Date(value).toLocaleString([], { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" });
}

function formatNoteListTitle(note) {
  return `note-${formatLocalTimestampForName(note.created_at)}`;
}

function formatLocalTimestampForName(value) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "unknown";
  const year = date.getFullYear();
  const month = padDatePart(date.getMonth() + 1);
  const day = padDatePart(date.getDate());
  const hour = padDatePart(date.getHours());
  const minute = padDatePart(date.getMinutes());
  const second = padDatePart(date.getSeconds());
  return `${year}-${month}-${day}-${hour}-${minute}-${second}${localTimezoneAbbreviation(date)}`;
}

function localTimezoneAbbreviation(date) {
  try {
    const part = new Intl.DateTimeFormat([], { timeZoneName: "short" })
      .formatToParts(date)
      .find((entry) => entry.type === "timeZoneName");
    const cleaned = String(part?.value || "").replace(/[^A-Za-z0-9]/g, "");
    return cleaned || "UTC";
  } catch {
    return "UTC";
  }
}

function padDatePart(value) {
  return `${value}`.padStart(2, "0");
}

function toggleAllVisibleNotes(event) {
  visibleNoteIds().forEach((noteId) => state.selectedNoteIds[event.target.checked ? "add" : "delete"](noteId));
  renderNotes();
}

function setNoteSelected(noteId, selected) {
  if (selected) {
    state.selectedNoteIds.add(noteId);
  } else {
    state.selectedNoteIds.delete(noteId);
  }
  updateBulkActions();
}

function visibleNoteIds() {
  return Array.from(document.querySelectorAll(".note-card"))
    .map((card) => card.dataset.noteId)
    .filter(Boolean);
}

function pruneSelectedNotes() {
  const currentNoteIds = new Set(state.notes.map((note) => note.note_id));
  state.selectedNoteIds.forEach((noteId) => {
    if (!currentNoteIds.has(noteId)) state.selectedNoteIds.delete(noteId);
  });
}

function updateBulkActions() {
  const visibleIds = visibleNoteIds();
  const selectedVisibleCount = visibleIds.filter((noteId) => state.selectedNoteIds.has(noteId)).length;
  const selectedCount = state.selectedNoteIds.size;
  const selectAll = document.getElementById("select-all-notes");
  selectAll.checked = visibleIds.length > 0 && selectedVisibleCount === visibleIds.length;
  selectAll.indeterminate = selectedVisibleCount > 0 && selectedVisibleCount < visibleIds.length;
  document.getElementById("selection-count").textContent = `${selectedCount} selected`;
  document.getElementById("bulk-delete-button").disabled = selectedCount === 0;
}

function formatDuration(ms) {
  if (!ms) return "0:00";
  const totalSeconds = Math.round(ms / 1000);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = `${totalSeconds % 60}`.padStart(2, "0");
  return `${minutes}:${seconds}`;
}

function formatSpeakerLabel(speakerId) {
  const raw = String(speakerId || "").trim();
  if (!raw) return "";
  const speakerMatch = raw.match(/^SPEAKER_0*(\d+)$/i);
  if (speakerMatch) return `Speaker ${Number(speakerMatch[1]) + 1}`;
  if (/^\d+$/.test(raw)) return `Speaker ${raw}`;
  return raw;
}

function escapeHtml(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

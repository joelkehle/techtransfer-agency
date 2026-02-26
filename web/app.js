(function () {
  "use strict";

  // DOM references
  var viewSubmit = document.getElementById("view-submit");
  var viewStatus = document.getElementById("view-status");
  var dropZone = document.getElementById("drop-zone");
  var fileInput = document.getElementById("file-input");
  var fileNameEl = document.getElementById("file-name");
  var workflowCheckboxes = document.getElementById("workflow-checkboxes");
  var submitError = document.getElementById("submit-error");
  var btnSubmit = document.getElementById("btn-submit");
  var statusToken = document.getElementById("status-token");
  var workflowStatusList = document.getElementById("workflow-status-list");
  var statusError = document.getElementById("status-error");
  var btnNew = document.getElementById("btn-new");
  var reportViewer = document.getElementById("report-viewer");
  var reportTitle = document.getElementById("report-title");
  var reportBadges = document.getElementById("report-badges");
  var reportHtml = document.getElementById("report-html");

  var selectedFile = null;
  var workflows = [];
  var pollTimer = null;

  // --- Workflow Fetching ---

  function fetchWorkflows() {
    fetch("/workflows")
      .then(function (res) {
        if (!res.ok) throw new Error("Failed to load workflows (HTTP " + res.status + ")");
        return res.json();
      })
      .then(function (data) {
        workflows = data.workflows || [];
        renderWorkflows();
      })
      .catch(function (err) {
        workflowCheckboxes.innerHTML =
          '<p class="error-msg">' + escapeHtml(err.message) + "</p>";
      });
  }

  function renderWorkflows() {
    if (workflows.length === 0) {
      workflowCheckboxes.innerHTML =
        '<p class="loading-msg">No workflows available.</p>';
      return;
    }

    var html = "";
    for (var i = 0; i < workflows.length; i++) {
      var wf = workflows[i];
      if (wf.available === false) continue;
      html +=
        '<label class="workflow-option">' +
        '<input type="checkbox" name="workflow" value="' + escapeAttr(wf.capability) + '">' +
        "<div>" +
        '<div class="wf-label">' + escapeHtml(wf.label || wf.capability) + "</div>" +
        (wf.description ? '<div class="wf-desc">' + escapeHtml(wf.description) + "</div>" : "") +
        "</div>" +
        "</label>";
    }

    if (!html) {
      workflowCheckboxes.innerHTML =
        '<p class="loading-msg">No workflows available.</p>';
      return;
    }

    workflowCheckboxes.innerHTML = html;

    // Listen for checkbox changes to update submit button state
    var boxes = workflowCheckboxes.querySelectorAll('input[type="checkbox"]');
    for (var j = 0; j < boxes.length; j++) {
      boxes[j].addEventListener("change", updateSubmitState);
    }
  }

  // --- File Handling ---

  dropZone.addEventListener("click", function () {
    fileInput.click();
  });

  dropZone.addEventListener("keydown", function (e) {
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      fileInput.click();
    }
  });

  fileInput.addEventListener("change", function () {
    if (fileInput.files.length > 0) {
      setFile(fileInput.files[0]);
    }
  });

  dropZone.addEventListener("dragover", function (e) {
    e.preventDefault();
    dropZone.classList.add("dragover");
  });

  dropZone.addEventListener("dragleave", function () {
    dropZone.classList.remove("dragover");
  });

  dropZone.addEventListener("drop", function (e) {
    e.preventDefault();
    dropZone.classList.remove("dragover");
    if (e.dataTransfer.files.length > 0) {
      setFile(e.dataTransfer.files[0]);
    }
  });

  function setFile(file) {
    selectedFile = file;
    fileNameEl.textContent = file.name;
    dropZone.classList.add("has-file");
    updateSubmitState();
  }

  // --- Submit Button State ---

  function updateSubmitState() {
    var anyChecked = workflowCheckboxes.querySelector(
      'input[type="checkbox"]:checked'
    );
    btnSubmit.disabled = !(selectedFile && anyChecked);
  }

  // --- Submission ---

  btnSubmit.addEventListener("click", function () {
    handleSubmit();
  });

  function handleSubmit() {
    hideError(submitError);

    var checked = workflowCheckboxes.querySelectorAll(
      'input[type="checkbox"]:checked'
    );
    if (!selectedFile || checked.length === 0) return;

    var selectedWorkflows = [];
    for (var i = 0; i < checked.length; i++) {
      selectedWorkflows.push(checked[i].value);
    }

    var formData = new FormData();
    formData.append("file", selectedFile);
    formData.append("workflows", selectedWorkflows.join(","));

    btnSubmit.disabled = true;
    btnSubmit.textContent = "Submitting...";

    fetch("/submit", {
      method: "POST",
      body: formData,
    })
      .then(function (res) {
        if (!res.ok) throw new Error("Submission failed (HTTP " + res.status + ")");
        return res.json();
      })
      .then(function (data) {
        if (!data.token) throw new Error("No token returned from server");
        showStatusView(data.token, selectedWorkflows);
      })
      .catch(function (err) {
        showError(submitError, err.message);
        btnSubmit.disabled = false;
        btnSubmit.textContent = "Submit";
      });
  }

  // --- Status View ---

  function showStatusView(token, selectedWorkflows) {
    viewSubmit.hidden = true;
    viewStatus.hidden = false;
    statusToken.textContent = token;

    // Build initial status list
    var html = "";
    for (var i = 0; i < selectedWorkflows.length; i++) {
      var cap = selectedWorkflows[i];
      var label = getWorkflowLabel(cap);
      html +=
        '<li class="wf-status-item" data-workflow="' + escapeAttr(cap) + '">' +
        '<div class="wf-status-left">' +
        '<div class="status-icon pending" data-icon>' + pendingIcon() + "</div>" +
        '<span class="wf-status-name">' + escapeHtml(label) + "</span>" +
        "</div>" +
        '<span class="status-label" data-action>Pending</span>' +
        "</li>";
    }
    workflowStatusList.innerHTML = html;

    // Start polling
    pollStatus(token);
  }

  function getWorkflowLabel(capability) {
    for (var i = 0; i < workflows.length; i++) {
      if (workflows[i].capability === capability) {
        return workflows[i].label || capability;
      }
    }
    return capability;
  }

  function pollStatus(token) {
    if (pollTimer) clearInterval(pollTimer);

    // Immediate first poll
    doStatusPoll(token);

    pollTimer = setInterval(function () {
      doStatusPoll(token);
    }, 3000);
  }

  function doStatusPoll(token) {
    fetch("/status/" + encodeURIComponent(token))
      .then(function (res) {
        if (!res.ok) throw new Error("Status check failed (HTTP " + res.status + ")");
        return res.json();
      })
      .then(function (data) {
        hideError(statusError);
        updateStatusUI(data);

        // Stop polling if everything is done
        if (isAllDone(data)) {
          clearInterval(pollTimer);
          pollTimer = null;
        }
      })
      .catch(function (err) {
        showError(statusError, err.message);
      });
  }

  function updateStatusUI(data) {
    var wfMap = data.workflows || {};
    var items = workflowStatusList.querySelectorAll(".wf-status-item");

    for (var i = 0; i < items.length; i++) {
      var item = items[i];
      var cap = item.getAttribute("data-workflow");
      var info = wfMap[cap];
      if (!info) continue;

      var iconEl = item.querySelector("[data-icon]");
      var actionEl = item.querySelector("[data-action]");

      var status = info.status || "submitted";

      // Update icon
      iconEl.className = "status-icon " + status;
      if (status === "completed") {
        iconEl.innerHTML = checkIcon();
      } else if (status === "executing") {
        iconEl.innerHTML = '<div class="spinner"></div>';
      } else if (status === "error") {
        iconEl.innerHTML = errorIcon();
      } else {
        iconEl.innerHTML = pendingIcon();
      }

      // Update action area
      if (status === "completed" && info.ready) {
        actionEl.innerHTML =
          '<button class="btn-view" data-token="' +
          escapeAttr(data.token) +
          '" data-workflow="' +
          escapeAttr(cap) +
          '">View</button> ' +
          '<a class="btn-download" href="/report/' +
          encodeURIComponent(data.token) +
          "/" +
          encodeURIComponent(cap) +
          '" download>Download</a>';
        actionEl.className = "";
      } else if (status === "error") {
        actionEl.textContent = "Error";
        actionEl.className = "status-label error";
      } else if (status === "executing") {
        actionEl.textContent = "In progress";
        actionEl.className = "status-label";
      } else {
        actionEl.textContent = "Pending";
        actionEl.className = "status-label";
      }
    }

    var viewButtons = workflowStatusList.querySelectorAll(".btn-view");
    for (var j = 0; j < viewButtons.length; j++) {
      if (viewButtons[j].getAttribute("data-bound") === "1") continue;
      viewButtons[j].setAttribute("data-bound", "1");
      viewButtons[j].addEventListener("click", function (e) {
        var token = e.currentTarget.getAttribute("data-token");
        var workflow = e.currentTarget.getAttribute("data-workflow");
        loadReportPreview(token, workflow);
      });
    }
  }

  function isAllDone(data) {
    var wfMap = data.workflows || {};
    for (var key in wfMap) {
      if (!wfMap.hasOwnProperty(key)) continue;
      var s = wfMap[key].status;
      if (s !== "completed" && s !== "error") return false;
    }
    return true;
  }

  // --- New Submission ---

  btnNew.addEventListener("click", function () {
    resetForm();
    viewStatus.hidden = true;
    viewSubmit.hidden = false;
  });

  function resetForm() {
    if (pollTimer) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
    selectedFile = null;
    fileInput.value = "";
    fileNameEl.textContent = "";
    dropZone.classList.remove("has-file");
    caseIdInput.value = "";
    btnSubmit.disabled = true;
    btnSubmit.textContent = "Submit";
    hideError(submitError);
    hideError(statusError);
    reportViewer.hidden = true;
    reportHtml.innerHTML = "";
    reportBadges.innerHTML = "";
    reportTitle.textContent = "Report Preview";

    var boxes = workflowCheckboxes.querySelectorAll('input[type="checkbox"]');
    for (var i = 0; i < boxes.length; i++) {
      boxes[i].checked = false;
    }
  }

  // --- Helpers ---

  function showError(el, msg) {
    el.textContent = msg;
    el.hidden = false;
  }

  function hideError(el) {
    el.textContent = "";
    el.hidden = true;
  }

  function escapeHtml(str) {
    var div = document.createElement("div");
    div.appendChild(document.createTextNode(str));
    return div.innerHTML;
  }

  function escapeAttr(str) {
    return str
      .replace(/&/g, "&amp;")
      .replace(/"/g, "&quot;")
      .replace(/'/g, "&#39;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;");
  }

  function pendingIcon() {
    return (
      '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">' +
      '<circle cx="12" cy="12" r="10"/>' +
      "</svg>"
    );
  }

  function checkIcon() {
    return (
      '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">' +
      '<path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/>' +
      '<polyline points="22 4 12 14.01 9 11.01"/>' +
      "</svg>"
    );
  }

  function errorIcon() {
    return (
      '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">' +
      '<circle cx="12" cy="12" r="10"/>' +
      '<line x1="15" y1="9" x2="9" y2="15"/>' +
      '<line x1="9" y1="9" x2="15" y2="15"/>' +
      "</svg>"
    );
  }

  function loadReportPreview(token, workflow) {
    fetch("/report/" + encodeURIComponent(token) + "/" + encodeURIComponent(workflow))
      .then(function (res) {
        if (!res.ok) throw new Error("Report fetch failed (HTTP " + res.status + ")");
        return res.text();
      })
      .then(function (raw) {
        var title = getWorkflowLabel(workflow) + " Report";
        var markdown = raw;
        var badges = [];

        try {
          var parsed = JSON.parse(raw);
          if (parsed && parsed.report_markdown) {
            markdown = parsed.report_markdown;
            if (parsed.determination) badges.push(parsed.determination);
            if (parsed.pathway) badges.push(parsed.pathway);
            if (parsed.recommendation) badges.push(parsed.recommendation);
            if (parsed.recommendation_confidence) badges.push(parsed.recommendation_confidence);
            if (parsed.report_mode) badges.push(parsed.report_mode);
          }
        } catch (e) {}

        reportTitle.textContent = title;
        reportBadges.innerHTML = badges
          .map(function (b) {
            return '<span class="report-badge">' + escapeHtml(String(b)) + "</span>";
          })
          .join("");
        reportHtml.innerHTML = markdownToHtml(markdown);
        reportViewer.hidden = false;
        reportViewer.scrollIntoView({ behavior: "smooth", block: "start" });
      })
      .catch(function (err) {
        showError(statusError, err.message);
      });
  }

  function markdownToHtml(markdown) {
    var lines = String(markdown || "").split("\n");
    var html = [];
    var inCode = false;
    var inUl = false;
    var inOl = false;
    var inBlockquote = false;
    var inTable = false;
    var tableHeaderDone = false;

    function closeLists() {
      if (inUl) {
        html.push("</ul>");
        inUl = false;
      }
      if (inOl) {
        html.push("</ol>");
        inOl = false;
      }
    }

    function closeQuote() {
      if (inBlockquote) {
        html.push("</blockquote>");
        inBlockquote = false;
      }
    }

    function closeTable() {
      if (inTable) {
        if (tableHeaderDone) {
          html.push("</tbody>");
        }
        html.push("</table>");
        inTable = false;
        tableHeaderDone = false;
      }
    }

    for (var i = 0; i < lines.length; i++) {
      var line = lines[i];
      var trimmed = line.trim();

      if (/^```/.test(trimmed)) {
        closeLists();
        closeQuote();
        closeTable();
        if (!inCode) {
          html.push('<pre><code>');
          inCode = true;
        } else {
          html.push("</code></pre>");
          inCode = false;
        }
        continue;
      }

      if (inCode) {
        html.push(escapeHtml(line) + "\n");
        continue;
      }

      if (trimmed === "") {
        closeLists();
        closeQuote();
        closeTable();
        continue;
      }

      if (/^---+$/.test(trimmed)) {
        closeLists();
        closeQuote();
        closeTable();
        html.push("<hr>");
        continue;
      }

      if (/^>\s?/.test(trimmed)) {
        closeLists();
        closeTable();
        if (!inBlockquote) {
          html.push("<blockquote>");
          inBlockquote = true;
        }
        html.push("<p>" + renderInline(trimmed.replace(/^>\s?/, "")) + "</p>");
        continue;
      } else {
        closeQuote();
      }

      if (/^###\s+/.test(trimmed)) {
        closeLists();
        closeTable();
        html.push("<h3>" + renderInline(trimmed.replace(/^###\s+/, "")) + "</h3>");
        continue;
      }
      if (/^##\s+/.test(trimmed)) {
        closeLists();
        closeTable();
        html.push("<h2>" + renderInline(trimmed.replace(/^##\s+/, "")) + "</h2>");
        continue;
      }
      if (/^#\s+/.test(trimmed)) {
        closeLists();
        closeTable();
        html.push("<h1>" + renderInline(trimmed.replace(/^#\s+/, "")) + "</h1>");
        continue;
      }

      if (/^\|.*\|$/.test(trimmed)) {
        closeLists();
        closeQuote();
        var cells = trimmed
          .split("|")
          .slice(1, -1)
          .map(function (c) {
            return renderInline(c.trim());
          });
        if (!inTable) {
          html.push('<table class="report-table">');
          inTable = true;
        }
        if (!tableHeaderDone) {
          html.push("<thead><tr>" + cells.map(function (c) { return "<th>" + c + "</th>"; }).join("") + "</tr></thead><tbody>");
          tableHeaderDone = true;
          continue;
        }
        if (/^[-:\s|]+$/.test(trimmed)) {
          continue;
        }
        html.push("<tr>" + cells.map(function (c) { return "<td>" + c + "</td>"; }).join("") + "</tr>");
        continue;
      }

      closeTable();

      if (/^[-*]\s+/.test(trimmed)) {
        if (!inUl) {
          closeLists();
          html.push("<ul>");
          inUl = true;
        }
        html.push("<li>" + renderInline(trimmed.replace(/^[-*]\s+/, "")) + "</li>");
        continue;
      }

      if (/^\d+\.\s+/.test(trimmed)) {
        if (!inOl) {
          closeLists();
          html.push("<ol>");
          inOl = true;
        }
        html.push("<li>" + renderInline(trimmed.replace(/^\d+\.\s+/, "")) + "</li>");
        continue;
      }

      closeLists();
      html.push("<p>" + renderInline(trimmed) + "</p>");
    }

    closeLists();
    closeQuote();
    closeTable();
    if (inCode) html.push("</code></pre>");
    return html.join("\n");
  }

  function renderInline(s) {
    var out = escapeHtml(s);
    out = out.replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>');
    out = out.replace(/`([^`]+)`/g, "<code>$1</code>");
    out = out.replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>");
    out = out.replace(/\*([^*]+)\*/g, "<em>$1</em>");
    return out;
  }

  // --- Init ---
  fetchWorkflows();
})();

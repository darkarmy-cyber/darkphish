$(function () {
    var currentStatus = null, busy = false, watching = false;
    var dialogStage = "closed", targetVersion = "", previous = {}, accepted = false, observedApplying = false;
    var diagnostics = {
        verification_failed: "Release download, authenticity or archive verification failed. No application files were changed.",
        backup_failed: "The pre-update backup failed. No application files were changed.",
        apply_failed: "Installation could not start. The backup completed and no application files were changed.",
        rollback: "Installation or startup failed. The previous application and database were restored."
    };

    function request(url, method, data) {
        return $.ajax({url: "/api" + url, method: method, data: data === undefined ? undefined : JSON.stringify(data),
            dataType: "json", contentType: "application/json", timeout: url === "/updates" || url === "/updates/check" ? 60000 : 15000});
    }

    function controls() {
        var hasUpdate = currentStatus && !currentStatus.error && !currentStatus.unsupported_reason &&
            currentStatus.latest_version && currentStatus.latest_version !== currentStatus.current_version;
        var reason = "";
        if (busy || watching) reason = "An update operation is in progress.";
        else if (!currentStatus) reason = "Check release information before updating.";
        else if (currentStatus.applying) reason = "Waiting for DarkPhish to restart.";
        else if (currentStatus.unsupported_reason) reason = currentStatus.unsupported_reason;
        else if (currentStatus.error) reason = currentStatus.error;
        else if (!hasUpdate) reason = "You are running the latest version.";
        $("#updateApply").prop("disabled", !!reason);
        $("#updateCheck").prop("disabled", busy || watching || !!(currentStatus && currentStatus.applying));
        if (reason) {
            $("#updateApplyHint").text(reason);
            $("#updateApplyLabel").text("Update now");
        } else {
            targetVersion = currentStatus.latest_version;
            $("#updateApplyLabel").text("Update to " + targetVersion);
            $("#updateApplyHint").text("Ready to install " + targetVersion + ".");
        }
    }

    function render(status) {
        if (status.unsupported_reason) status.releases = [];
        currentStatus = status;
        $("#updateCurrent").text(status.current_version);
        $("#updateLatest").text(status.latest_version || "—");
        var hasUpdate = status.latest_version && status.latest_version !== status.current_version && !status.error && !status.unsupported_reason;
        $("#updateAvailableBadge").prop("hidden", !hasUpdate);
        if (hasUpdate) {
            $("#updateAvailableText").text("Version " + status.latest_version + " is available!");
            targetVersion = status.latest_version;
        }
        $("#updateStatus").text(status.result || status.unsupported_reason || status.error || (status.applying ? "Update in progress" : hasUpdate ? "A new release is available" : "You have the latest version"));
        $("#updateBlocked").prop("hidden", !status.unsupported_reason);
        $("#updateBlockedReason").text(status.unsupported_reason || "");
        if (status.releases && status.releases.length > 0) {
            var latest = status.releases[0];
            $("#updateNotes").text(latest.release_notes || "No release notes available.");
            $("#updatePublished").text(latest.published_at || "—");
            $("#updateNotesSection").prop("hidden", false);
        } else {
            $("#updateNotesSection").prop("hidden", true);
        }
        controls();
    }

    function openProgress() {
        if (dialogStage === "closed") {
            Swal.fire({title: "Updating DarkPhish", customClass: "update-progress-dialog", showConfirmButton: false,
                allowOutsideClick: function () { return dialogStage !== "progress"; },
                allowEscapeKey: function () { return dialogStage !== "progress"; },
                onClose: function () { dialogStage = "closed"; }});
        }
        dialogStage = "progress";
    }

    function progress(message) {
        openProgress();
        Swal.update({title: "Updating DarkPhish", showConfirmButton: false, showCancelButton: false,
            html: '<p id="updateDialogMessage" role="status" aria-live="polite"></p><div class="update-progress-track"></div><p>Keep this window open. The application may restart.</p>'});
        $("#updateDialogMessage").text(message);
        $("#updateStatus").text(message);
    }

    function finish(kind, message, detail) {
        watching = false; busy = false;
        openProgress(); dialogStage = "finished";
        Swal.update({title: kind === "success" ? "Update successful" : kind === "error" ? "Update failed" : "Update not confirmed",
            type: kind, showConfirmButton: true, confirmButtonText: "Close",
            html: '<p id="updateDialogMessage"></p><pre id="updateDialogDiagnostics"></pre>'});
        $("#updateDialogMessage").text(message);
        $("#updateDialogDiagnostics").text(detail || "").prop("hidden", !detail);
        Swal.enableButtons();
        $("#updateStatus").text(message);
        controls();
    }

    function unknown(message) {
        currentStatus = null;
        finish("info", message, "Reconnect and check Settings > Update for the result.");
    }

    function outcome(status) {
        if (status.applying) { observedApplying = true; targetVersion = status.target_version || targetVersion; return false; }
        var fresh = accepted || observedApplying || status.result !== previous.result || status.current_version !== previous.current_version;
        if (!fresh) return false;
        if (status.result_code === "applied" && targetVersion && status.current_version === targetVersion) {
            finish("success", "DarkPhish " + status.current_version + " is running. " + status.result);
            return true;
        }
        if (Object.prototype.hasOwnProperty.call(diagnostics, status.result_code)) {
            finish("error", status.result || status.error, diagnostics[status.result_code]);
            return true;
        }
        if (status.result && status.result !== previous.result && /failed|rollback/i.test(status.result)) {
            finish("error", status.result, "Inspect the DarkPhish service log for details.");
            return true;
        }
        return false;
    }

    function watchRestart(remaining) {
        if (remaining <= 0) { unknown("Update is taking longer than expected."); return; }
        watching = true; controls();
        setTimeout(function () {
            request("/updates", "GET").done(function (status) {
                render(status);
                if (outcome(status)) return;
                progress(status.applying ? "Preparing the update…" : "Waiting for the update result…");
                watchRestart(remaining - 1);
            }).fail(function (xhr) {
                if (xhr.status === 401 || xhr.status === 403) { unknown("Sign in again to verify."); return; }
                progress("Waiting for DarkPhish to restart…");
                watchRestart(remaining - 1);
            });
        }, 3000);
    }

    function failureMessage(xhr) { return xhr.responseJSON && xhr.responseJSON.message || "Unable to complete the update"; }

    function recover(xhr, mayHaveApplied) {
        request("/updates", "GET").done(function (status) {
            render(status);
            if (status.applying) { observedApplying = true; progress("An update is already running."); watchRestart(240); return; }
            if (mayHaveApplied) {
                if (outcome(status)) return;
                unknown(status.error || "The update was interrupted.");
            } else {
                finish("error", failureMessage(xhr), "The request was rejected.");
            }
        }).fail(function () {
            if (mayHaveApplied) { progress("Waiting for restart…"); watchRestart(240); }
            else { currentStatus = null; finish("error", failureMessage(xhr), "Check the connection."); }
        });
    }

    function check(force) {
        busy = true; controls();
        return request(force ? "/updates/check" : "/updates", force ? "POST" : "GET").done(render).fail(function (xhr) {
            currentStatus = null; $("#updateStatus").text(failureMessage(xhr));
        }).always(function () { busy = false; controls(); });
    }

    function begin(password) {
        progress("Verifying administrator access…");
        var proof = {method: "password", password: password};
        var auth = request("/reauthenticate", "POST", proof);
        proof.password = "";
        auth.done(function () {
            progress("Requesting update…");
            request("/updates/apply", "POST", {version: targetVersion}).done(function (response) {
                targetVersion = response.target_version || targetVersion;
                accepted = true; currentStatus.applying = true;
                progress("Preparing the update…");
                watchRestart(240);
            }).fail(function (xhr) { recover(xhr, !(xhr.status >= 400 && xhr.status < 500)); });
        }).fail(function (xhr) { recover(xhr, false); });
    }

    $("#updateCheck").on("click", function () {
        if (busy || watching || (currentStatus && currentStatus.applying)) return;
        check(true).done(function (status) {
            if (status.applying) { previous = {}; targetVersion = status.target_version || status.latest_version; progress("Resuming update…"); watchRestart(240); }
        });
    });

    $("#updateApply").on("click", function () {
        if (busy || watching || !currentStatus || !targetVersion || currentStatus.unsupported_reason || currentStatus.error || currentStatus.applying) return;
        previous = Object.assign({}, currentStatus);
        accepted = false; observedApplying = false; busy = true; dialogStage = "confirm"; controls();
        Swal.fire({title: "Update to " + targetVersion + "?",
            text: "Current: " + currentStatus.current_version + " → Target: " + targetVersion + ". Enter your password to update.",
            input: "password", customClass: "update-progress-dialog",
            showCancelButton: true, confirmButtonText: "Update",
            preConfirm: function (password) {
                if (dialogStage === "finished") return true;
                if (dialogStage !== "confirm" || !password) return false;
                begin(password); return false;
            },
            onClose: function () { dialogStage = "closed"; }
        }).then(function () { if (!watching && dialogStage !== "progress") { busy = false; controls(); } });
    });

    check(false).done(function (status) {
        if (status.applying) { previous = Object.assign({}, status); targetVersion = status.target_version || status.latest_version; progress("Resuming update…"); watchRestart(240); }
    });
});

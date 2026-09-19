$(function () {
    var currentStatus = null, busy = false, watching = false;
    var dialogStage = "closed", targetVersion = "", previous = {}, accepted = false, observedApplying = false;
    var diagnostics = {
        verification_failed: "Release download, authenticity or archive verification failed. No application files were changed.",
        backup_failed: "The pre-update backup failed. No application files were changed. Check available disk space and filesystem permissions.",
        apply_failed: "Installation could not start. The backup completed and no application files were changed.",
        rollback: "Installation or startup failed. The previous application and database were restored."
    };
    function request(url, method, data) {
        return $.ajax({url: "/api" + url, method: method, data: data === undefined ? undefined : JSON.stringify(data),
            dataType: "json", contentType: "application/json", timeout: 15000});
    }
    function controls() {
        var reason = busy || watching ? "An update operation is in progress." :
            !currentStatus ? "Check release information before updating." :
            currentStatus.applying ? "Waiting for DarkPhish to restart. Do not start another update." :
            currentStatus.unsupported_reason || currentStatus.error ||
            (!currentStatus.available ? "No newer stable release is available." : "");
        $("#updateApply").prop("disabled", !!reason);
        $("#updateCheck").prop("disabled", busy || watching || !!(currentStatus && currentStatus.applying));
        $("#updateApplyHint").text(reason || "Ready. Update now requires password confirmation and a verified backup.");
    }
    function render(status) {
        currentStatus = status;
        $("#updateCurrent").text(status.current_version);
        $("#updateLatest").text(status.latest_version || "Unavailable");
        $("#updatePublished").text(status.latest_version ? status.published_at : "—");
        $("#updateNotes").text(status.release_notes || "");
        $("#updateStatus").text(status.result || status.error || (status.applying ? "Update in progress" : status.available ? "A new stable release is available" : "You are up to date"));
        $("#updateBlocked").prop("hidden", !status.unsupported_reason);
        $("#updateBlockedReason").text(status.unsupported_reason || "");
        $("#updateVerifierHelp").prop("hidden", !/\/usr\/bin\/gh|GitHub CLI|attestation verifier/i.test(status.unsupported_reason || ""));
        controls();
        renderAdminNotification(status);
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
        var input = Swal.getInput();
        if (input) { input.value = ""; input.disabled = true; input.hidden = true; input.style.display = "none"; }
        Swal.update({title: "Updating DarkPhish", showConfirmButton: false, showCancelButton: false,
            html: '<p id="updateDialogMessage" class="update-progress-message" role="status" aria-live="polite"></p><div class="update-progress-track" role="progressbar" aria-label="Update in progress"></div><p>Keep this window open. The application may temporarily disconnect while restarting.</p>'});
        $("#updateDialogMessage").text(message);
        $("#updateStatus").text(message);
    }
    function finish(kind, message, detail) {
        watching = false; busy = false;
        openProgress(); dialogStage = "finished";
        Swal.update({title: kind === "success" ? "Update successful" : kind === "error" ? "Update failed" : "Update not confirmed",
            type: kind, showConfirmButton: true, showCancelButton: false, confirmButtonText: "Close",
            html: '<p id="updateDialogMessage" role="status" aria-live="polite"></p><pre id="updateDialogDiagnostics" class="update-progress-diagnostics"></pre>'});
        // Server-supplied content is text, never interpolated into dialog HTML.
        $("#updateDialogMessage").text(message);
        $("#updateDialogDiagnostics").text(detail || "").prop("hidden", !detail);
        // SweetAlert2 8 hides the actions container during progress but does not
        // restore the container when showConfirmButton is updated back to true.
        Swal.getActions().style.display = "flex";
        Swal.enableButtons();
        $("#updateStatus").text(message);
        controls();
    }
    function unknown(message) {
        currentStatus = null;
        finish("info", message, "The result is not known. Do not repeat the update blindly. Reconnect or sign in again, then open Settings > Update. If the service is unavailable, an administrator can inspect the DarkPhish service log (for systemd: journalctl -u darkphish.service -n 100 --no-pager). Remove secrets before sharing logs.");
    }
    function outcome(status) {
        if (status.applying) { observedApplying = true; return false; }
        var fresh = accepted || observedApplying || status.result !== previous.result || status.current_version !== previous.current_version;
        if (!fresh) return false;
        // An accepted request or a stale previous success is not a completed update.
        if ((status.result_code === "applied" || /^Update completed successfully\.?$/.test(status.result || "")) &&
            targetVersion && status.current_version === targetVersion && status.current_version !== previous.current_version) {
            finish("success", "DarkPhish " + status.current_version + " is running. " + status.result);
            return true;
        }
        if (Object.prototype.hasOwnProperty.call(diagnostics, status.result_code)) {
            finish("error", status.result || status.error, diagnostics[status.result_code] + "\nDiagnostic code: " + status.result_code + "\nFor the detailed cause, inspect the DarkPhish service log. Remove secrets before sharing logs.");
            return true;
        }
        if (status.result && status.result !== previous.result && /failed|restored|rollback|could not|backup/i.test(status.result)) {
            finish("error", status.result, "Inspect the DarkPhish service log for the detailed cause. Remove secrets before sharing logs.");
            return true;
        }
        return false;
    }
    function watchRestart(remaining) {
        if (remaining <= 0) { unknown("The update is taking longer than expected. Its outcome could not be confirmed."); return; }
        watching = true; controls();
        setTimeout(function () {
            request("/updates", "GET").done(function (status) {
                render(status);
                if (outcome(status)) return;
                progress(status.applying ? "Verifying the release and preparing the update…" : "Waiting for the update result…");
                watchRestart(remaining - 1);
            }).fail(function (xhr) {
                if (xhr.status === 401 || xhr.status === 403) { unknown("Sign in again to verify the update result."); return; }
                progress("Waiting for DarkPhish to restart…");
                watchRestart(remaining - 1);
            });
        }, 3000);
    }
    function failureMessage(xhr) { return xhr.responseJSON && xhr.responseJSON.message || "Unable to complete the update operation"; }
    function recover(xhr, mayHaveApplied) {
        // GET only: an interrupted POST may have reached the supervisor. Never replay it.
        request("/updates", "GET").done(function (status) {
            render(status);
            if (mayHaveApplied) {
                if (outcome(status)) return;
                if (status.applying) { progress("Verifying the release and preparing the update…"); watchRestart(240); return; }
                unknown(status.error || "The update request was interrupted. Its outcome could not be confirmed.");
            } else {
                finish("error", failureMessage(xhr), "The request was rejected. No update was started by this request.");
            }
        }).fail(function () {
            if (mayHaveApplied) { progress("Waiting for DarkPhish to restart…"); watchRestart(240); }
            else { currentStatus = null; finish("error", failureMessage(xhr), "The request was rejected. Check the connection and sign in again if necessary."); }
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
            progress("Requesting a verified update…");
            request("/updates/apply", "POST", {}).done(function () {
                accepted = true; currentStatus.applying = true;
                progress("Verifying the release and preparing the update…");
                watchRestart(240);
            }).fail(function (xhr) { recover(xhr, !(xhr.status >= 400 && xhr.status < 500)); });
        }).fail(function (xhr) { recover(xhr, false); });
    }
    $("#updateCheck").on("click", function () {
        if (busy || watching || (currentStatus && currentStatus.applying)) return;
        check(true).done(function (status) {
            if (status.applying) { previous = {}; targetVersion = status.latest_version; progress("Resuming update monitoring…"); watchRestart(240); }
        });
    });
    $("#updateApply").on("click", function () {
        if (busy || watching || !currentStatus || !currentStatus.available || currentStatus.unsupported_reason || currentStatus.error || currentStatus.applying) return;
        previous = Object.assign({}, currentStatus); targetVersion = currentStatus.latest_version;
        accepted = false; observedApplying = false; busy = true; dialogStage = "confirm"; controls();
        Swal.fire({title: "Confirm privileged access", text: "Enter your password to back up and update DarkPhish. The application will restart.",
            input: "password", inputAttributes: {autocomplete: "current-password"}, customClass: "update-progress-dialog",
            showCancelButton: true, confirmButtonText: "Verify and update",
            allowOutsideClick: function () { return dialogStage !== "progress"; },
            allowEscapeKey: function () { return dialogStage !== "progress"; },
            preConfirm: function (password) {
                if (dialogStage === "finished") return true;
                if (dialogStage !== "confirm" || !password) return false;
                begin(password); return false; // Retain this dialog through restart and completion.
            },
            onClose: function () { dialogStage = "closed"; }
        }).then(function () { if (!watching && dialogStage !== "progress") { busy = false; controls(); } });
    });
    check(false).done(function (status) {
        if (status.applying) { previous = Object.assign({}, status); targetVersion = status.latest_version; progress("Resuming update monitoring…"); watchRestart(240); }
    });
});

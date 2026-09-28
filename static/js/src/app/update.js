$(function () {
    var currentStatus = null, busy = false, watching = false;
    var dialogStage = "closed", targetVersion = "", selectedVersion = "", previous = {}, accepted = false, observedApplying = false;
    var diagnostics = {
        verification_failed: "Release download, authenticity or archive verification failed. No application files were changed.",
        backup_failed: "The pre-update backup failed. No application files were changed. Check available disk space and filesystem permissions.",
        apply_failed: "Installation could not start. The backup completed and no application files were changed.",
        rollback: "Installation or startup failed. The previous application and database were restored."
    };

    function request(url, method, data) {
        return $.ajax({url: "/api" + url, method: method, data: data === undefined ? undefined : JSON.stringify(data),
            dataType: "json", contentType: "application/json", timeout: url === "/updates" || url === "/updates/check" ? 60000 : 15000});
    }

    function releases() {
        return currentStatus && Array.isArray(currentStatus.releases) ? currentStatus.releases : [];
    }

    function releaseFor(version) {
        var list = releases();
        for (var i = 0; i < list.length; i++) if (list[i].version === version) return list[i];
        return null;
    }

    function selectedRelease() {
        var select = document.getElementById("updateTarget");
        var value = select && select.value ? select.value : selectedVersion;
        return releaseFor(value);
    }

    function actionLabel(release) {
        if (!release) return "Update now";
        if (release.action === "reinstall") return "Reinstall " + release.version;
        if (release.action === "downgrade") return "Downgrade to " + release.version;
        return "Update to " + release.version;
    }

    function renderSelectedRelease() {
        var release = selectedRelease();
        if (!release) {
            $("#updatePublished").text("—");
            $("#updateNotes").text("");
            $("#updateTargetWarning").prop("hidden", true).text("");
            $("#updateApply").text("Update now");
            return;
        }
        selectedVersion = release.version;
        $("#updatePublished").text(release.published_at || "—");
        $("#updateNotes").text(release.release_notes || "");
        $("#updateApply").text(actionLabel(release));
        var warning = release.compatible ? "" : (release.disabled_reason || "This version cannot be installed on the current system.");
        $("#updateTargetWarning").prop("hidden", !warning).text(warning);
    }

    function renderSelector(status) {
        var select = document.getElementById("updateTarget");
        if (!select) return;
        while (select.firstChild) select.removeChild(select.firstChild);
        var list = Array.isArray(status.releases) ? status.releases : [];
        for (var i = 0; i < list.length; i++) {
            var release = list[i];
            var option = document.createElement("option");
            option.value = release.version;
            var suffix = release.latest ? " — Latest stable" : release.current ? " — Current" : "";
            option.textContent = release.version + suffix;
            option.disabled = release.compatible === false;
            select.appendChild(option);
        }
        var preferred = selectedVersion && releaseFor(selectedVersion) ? selectedVersion : status.selected_version || status.latest_version || "";
        select.value = preferred;
        if (!select.value && select.options && select.options.length) select.value = select.options[0].value;
        selectedVersion = select.value || "";
        renderSelectedRelease();
    }

    function controls() {
        var release = selectedRelease();
        var reason = busy || watching ? "An update operation is in progress." :
            !currentStatus ? "Check release information before updating." :
            currentStatus.applying ? "Waiting for DarkPhish to restart. Do not start another update." :
            currentStatus.unsupported_reason || currentStatus.error ||
            !release ? "No verified stable release is selected." :
            release.compatible === false ? (release.disabled_reason || "The selected version is not compatible with this installation.") : "";
        $("#updateApply").prop("disabled", !!reason);
        $("#updateCheck").prop("disabled", busy || watching || !!(currentStatus && currentStatus.applying));
        $("#updateTarget").prop("disabled", busy || watching || !!(currentStatus && currentStatus.applying) || releases().length === 0);
        if (reason) $("#updateApplyHint").text(reason);
        else if (release.action === "reinstall") $("#updateApplyHint").text("Ready to reinstall " + release.version + ". Password confirmation and a verified backup are required.");
        else $("#updateApplyHint").text("Ready to install " + release.version + ". Password confirmation and a verified backup are required.");
    }

    function render(status) {
        currentStatus = status;
        $("#updateCurrent").text(status.current_version);
        if (status.applying && status.target_version) selectedVersion = status.target_version;
        renderSelector(status);
        $("#updateStatus").text(status.result || status.error || (status.applying ? "Update in progress" : status.available ? "A new stable release is available" : "Stable release information is current"));
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
        $("#updateDialogMessage").text(message);
        $("#updateDialogDiagnostics").text(detail || "").prop("hidden", !detail);
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
        if (status.applying) { observedApplying = true; targetVersion = status.target_version || targetVersion; return false; }
        var fresh = accepted || observedApplying || status.result !== previous.result || status.current_version !== previous.current_version;
        if (!fresh) return false;
        var applied = status.result_code === "applied" || /^Update completed successfully\.?$/.test(status.result || "");
        var targetReceipt = status.target_version === targetVersion;
        var versionChanged = status.current_version !== previous.current_version;
        if (applied && targetVersion && status.current_version === targetVersion && (observedApplying || targetReceipt || versionChanged)) {
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
        request("/updates", "GET").done(function (status) {
            render(status);
            if (status.applying) { observedApplying = true; targetVersion = status.target_version || targetVersion; progress("An update is already running. Monitoring its result…"); watchRestart(240); return; }
            if (mayHaveApplied) {
                if (outcome(status)) return;
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
            request("/updates/apply", "POST", {version: targetVersion}).done(function (response) {
                targetVersion = response.target_version || targetVersion;
                accepted = true; currentStatus.applying = true;
                progress("Verifying the release and preparing the update…");
                watchRestart(240);
            }).fail(function (xhr) { recover(xhr, !(xhr.status >= 400 && xhr.status < 500)); });
        }).fail(function (xhr) { recover(xhr, false); });
    }

    $("#updateTarget").on("change", function () {
        selectedVersion = this.value || "";
        renderSelectedRelease();
        controls();
    });

    $("#updateCheck").on("click", function () {
        if (busy || watching || (currentStatus && currentStatus.applying)) return;
        check(true).done(function (status) {
            if (status.applying) { previous = {}; targetVersion = status.target_version || status.latest_version; progress("Resuming update monitoring…"); watchRestart(240); }
        });
    });

    $("#updateApply").on("click", function () {
        var release = selectedRelease();
        if (busy || watching || !currentStatus || !release || release.compatible === false || currentStatus.unsupported_reason || currentStatus.error || currentStatus.applying) return;
        previous = Object.assign({}, currentStatus); targetVersion = release.version;
        accepted = false; observedApplying = false; busy = true; dialogStage = "confirm"; controls();
        var verb = release.action === "reinstall" ? "reinstall" : release.action === "downgrade" ? "downgrade to" : "update to";
        Swal.fire({title: "Install DarkPhish " + release.version + "?",
            text: "Current version: " + currentStatus.current_version + ". Target version: " + release.version + ". Enter your password to " + verb + " this verified release. DarkPhish will create a backup and restart.",
            input: "password", inputAttributes: {autocomplete: "current-password"}, customClass: "update-progress-dialog",
            showCancelButton: true, confirmButtonText: release.action === "reinstall" ? "Verify and reinstall" : "Verify and update",
            allowOutsideClick: function () { return dialogStage !== "progress"; },
            allowEscapeKey: function () { return dialogStage !== "progress"; },
            preConfirm: function (password) {
                if (dialogStage === "finished") return true;
                if (dialogStage !== "confirm" || !password) return false;
                begin(password); return false;
            },
            onClose: function () { dialogStage = "closed"; }
        }).then(function () { if (!watching && dialogStage !== "progress") { busy = false; controls(); } });
    });

    check(false).done(function (status) {
        if (status.applying) { previous = Object.assign({}, status); targetVersion = status.target_version || status.latest_version; progress("Resuming update monitoring…"); watchRestart(240); }
    });
});

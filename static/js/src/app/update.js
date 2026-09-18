$(function () {
    var currentStatus = null;
    var busy = false;
    function controls() {
        var reason = busy ? "An update operation is in progress." :
            !currentStatus ? "Check release information before updating." :
            currentStatus.applying ? "Waiting for DarkPhish to restart. Do not start another update." :
            currentStatus.unsupported_reason || currentStatus.error ||
            (!currentStatus.available ? "No newer stable release is available." : "");
        $("#updateApply").prop("disabled", !!reason);
        $("#updateCheck").prop("disabled", busy || !!(currentStatus && currentStatus.applying));
        $("#updateApplyHint").text(reason || "Ready. Update now requires password confirmation and a verified backup.");
    }
    function render(status) {
        currentStatus = status;
        $("#updateCurrent").text(status.current_version);
        $("#updateLatest").text(status.latest_version || "Unavailable");
        $("#updatePublished").text(status.latest_version ? status.published_at : "—");
        $("#updateNotes").text(status.release_notes || "");
        $("#updateStatus").text(status.error || status.result || (status.applying ? "Update in progress" : status.available ? "A new stable release is available" : "You are up to date"));
        $("#updateBlocked").prop("hidden", !status.unsupported_reason);
        $("#updateBlockedReason").text(status.unsupported_reason || "");
        $("#updateVerifierHelp").prop("hidden", !/\/usr\/bin\/gh|GitHub CLI|attestation verifier/i.test(status.unsupported_reason || ""));
        controls();
        renderAdminNotification(status);
    }
    function failed(xhr) { $("#updateStatus").text(xhr.responseJSON && xhr.responseJSON.message || "Unable to complete the update operation"); }
    function recover(xhr) {
        // A failed response can still mean apply reached the supervisor. Recheck
        // server state before allowing a retry, without repeating the POST.
        check(false).done(function (status) {
            if (status.applying) watchRestart(100);
            else if (!status.result && !status.error) failed(xhr);
        });
    }
    function watchRestart(remaining) {
        if (remaining <= 0) { $("#updateStatus").text("The update is taking longer than expected. Reload this page to check its status."); return; }
        setTimeout(function () {
            query("/updates", "GET", undefined, true).done(function (status) {
                render(status);
                if (status.applying) watchRestart(remaining - 1);
            }).fail(function () {
                $("#updateStatus").text("Waiting for DarkPhish to restart…");
                watchRestart(remaining - 1);
            });
        }, 3000);
    }
    function check(force) {
        busy = true;
        controls();
        return query(force ? "/updates/check" : "/updates", force ? "POST" : "GET", undefined, true).done(render).fail(function (xhr) {
            currentStatus = null;
            failed(xhr);
        }).always(function () { busy = false; controls(); });
    }
    $("#updateCheck").on("click", function () { check(true); });
    $("#updateApply").on("click", function () {
        if (busy || !currentStatus || !currentStatus.available || currentStatus.unsupported_reason || currentStatus.error || currentStatus.applying) return;
        busy = true;
        controls();
        Swal.fire({title: "Confirm privileged access", text: "Enter your password to back up and update DarkPhish. The application will restart.", input: "password", showCancelButton: true, confirmButtonText: "Verify and update"}).then(function (result) {
            if (!result.value) { busy = false; controls(); return; }
            var proof = {method: "password", password: result.value};
            result.value = "";
            var request = query("/reauthenticate", "POST", proof, true);
            proof.password = "";
            request.done(function () {
                query("/updates/apply", "POST", {}, true).done(function (response) {
                    busy = false;
                    currentStatus.applying = true;
                    controls();
                    $("#updateStatus").text(response.message);
                    watchRestart(100);
                }).fail(recover);
            }).fail(recover);
        });
    });
    check(false).done(function (status) {
        if (status.applying) watchRestart(100);
    });
});

$(function () {
    function render(status) {
        $("#updateCurrent").text(status.current_version);
        $("#updateLatest").text(status.latest_version || "Unavailable");
        $("#updatePublished").text(status.latest_version ? status.published_at : "—");
        $("#updateNotes").text(status.release_notes || "");
        $("#updateStatus").text(status.error || status.result || status.unsupported_reason || (status.applying ? "Update in progress" : status.available ? "A new stable release is available" : "You are up to date"));
        $("#updateApply").prop("disabled", !status.available || !!status.unsupported_reason || !!status.error || status.applying);
        renderAdminNotification(status);
    }
    function failed(xhr) { $("#updateStatus").text(xhr.responseJSON && xhr.responseJSON.message || "Unable to complete the update operation"); }
    function watchRestart(remaining) {
        if (remaining <= 0) { $("#updateStatus").text("The update is taking longer than expected. Reload this page to check its status."); return; }
        setTimeout(function () {
            query("/updates", "GET", undefined, true).done(function (status) {
                render(status);
                if (status.applying) watchRestart(remaining - 1);
            }).fail(function () {
                $("#updateStatus").text("Waiting for Darkphish to restart…");
                watchRestart(remaining - 1);
            });
        }, 3000);
    }
    function check(force) {
        $("#updateCheck, #updateApply").prop("disabled", true);
        return query(force ? "/updates/check" : "/updates", force ? "POST" : "GET", undefined, true).done(render).fail(failed).always(function () { $("#updateCheck").prop("disabled", false); });
    }
    $("#updateCheck").on("click", function () { check(true); });
    $("#updateApply").on("click", function () {
        Swal.fire({title: "Confirm privileged access", text: "Enter your password to back up and update Darkphish. The application will restart.", input: "password", showCancelButton: true, confirmButtonText: "Verify and update"}).then(function (result) {
            if (!result.value) return;
            $("#updateApply").prop("disabled", true);
            var proof = {method: "password", password: result.value};
            result.value = "";
            var request = query("/reauthenticate", "POST", proof, true);
            proof.password = "";
            request.done(function () {
                query("/updates/apply", "POST", {}, true).done(function (response) { $("#updateStatus").text(response.message); watchRestart(100); }).fail(failed);
            }).fail(failed);
        });
    });
    check(false);
});

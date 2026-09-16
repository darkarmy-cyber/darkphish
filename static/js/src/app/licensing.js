$(document).ready(function () {
    var configured = false;
    function busy(value) { $("#licenseActivate, #licenseRefresh").prop("disabled", value || !configured); }
    function render(status) {
        configured = status.configured === true;
        $("#licenseState").text(status.state || "missing");
        $("#licenseID").text(status.license_id || "Not activated");
        $("#licenseInstallation").text(status.installation_id || "—");
        $("#licenseExpiry").text(status.expires_at ? new Date(status.expires_at * 1000).toLocaleString() : "—");
        $("#licenseUsers").text(status.managed_users + " / " + status.managed_users_limit);
        $("#licenseCampaigns").text(status.active_campaigns + " / " + status.active_campaigns_limit);
        $("#licenseMessage").text(configured ? (status.state === "grace" ? "Your license is in its offline grace period. Refresh or renew it before the grace period ends." : "License information updated.") : "Activation is unavailable. Your administrator must configure the licensing service.");
        busy(false);
    }
    function failed(xhr) { $("#licenseMessage").text(xhr.responseJSON && xhr.responseJSON.message || "Unable to contact the licensing service. Please try again later."); }
    $("#licenseActivation").on("submit", function (event) {
        event.preventDefault();
        if (!configured) return;
        var data = {license_key: $("#licenseKey").val().trim()};
        $("#licenseKey").val("");
        if (!data.license_key) return;
        busy(true);
        var request = query("/license/activate", "POST", data, true);
        data.license_key = "";
        request.done(render).fail(failed).always(function () { busy(false); });
    });
    $("#licenseRefresh").on("click", function () {
        busy(true);
        query("/license/refresh", "POST", {}, true).done(render).fail(failed).always(function () { busy(false); });
    });
    query("/license", "GET", undefined, true).done(render).fail(failed);
});

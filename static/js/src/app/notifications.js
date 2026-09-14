/* Release text is untrusted and must only enter text nodes. */
function renderAdminNotification(status) {
    var count = status.available === true && status.badge === 1 ? 1 : 0;
    $("#adminNotificationBadge").text(count || "").prop("hidden", count === 0);
    $("#adminReleaseNotification").text(count ? "Darkphish " + status.latest_version + " is available" : "No new stable release");
}
$(function () {
    if (!$("#adminNotifications").length) return;
    function refresh() {
        query("/updates", "GET", undefined, true).done(renderAdminNotification).fail(function () {
            $("#adminNotificationBadge").prop("hidden", true);
            $("#adminReleaseNotification").text("Release check unavailable");
        });
    }
    refresh();
    setInterval(refresh, 5 * 60 * 1000);
});

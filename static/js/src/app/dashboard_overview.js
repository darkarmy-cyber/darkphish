(function () {
    "use strict";

    function rate(count, total) {
        if (!total) return "0%";
        return Math.round((count / total) * 100) + "%";
    }

    function aggregate(items) {
        var totals = {
            campaigns: items.length,
            recipients: 0,
            sent: 0,
            opened: 0,
            clicked: 0,
            reported: 0
        };
        $.each(items, function (_index, campaign) {
            var stats = campaign.stats || {};
            totals.recipients += Number(stats.total || 0);
            totals.sent += Number(stats.sent || 0);
            totals.opened += Number(stats.opened || 0);
            totals.clicked += Number(stats.clicked || 0);
            totals.reported += Number(stats.email_reported || 0);
        });
        return totals;
    }

    function render() {
        if (!Array.isArray(window.campaigns)) return;

        var totals = aggregate(window.campaigns);
        $("#kpiCampaigns").text(String(totals.campaigns));
        $("#kpiRecipients").text(String(totals.recipients));
        $("#kpiSentRate").text(rate(totals.sent, totals.recipients));
        $("#kpiOpenRate").text(rate(totals.opened, totals.recipients));
        $("#kpiClickRate").text(rate(totals.clicked, totals.recipients));
        $("#kpiReportRate").text(rate(totals.reported, totals.recipients));
    }

    $(document).ajaxComplete(render);
    $(document).ready(function () {
        render();
        window.setTimeout(render, 250);
    });
})();

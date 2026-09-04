$(document).ready(function () {
    var page = 1
    var total = 0
    var perPage = 50

    function filters() {
        var values = {page: page, per_page: perPage}
        if ($("#audit_action").val()) values.action = $("#audit_action").val()
        if ($("#audit_actor").val()) values.actor = $("#audit_actor").val()
        if ($("#audit_actor_id").val()) values.actor_id = $("#audit_actor_id").val()
        if ($("#audit_target_type").val()) values.target_type = $("#audit_target_type").val()
        if ($("#audit_target_id").val()) values.target_id = $("#audit_target_id").val()
        if ($("#audit_result").val()) values.result = $("#audit_result").val()
        if ($("#audit_from").val()) values.from = moment($("#audit_from").val()).utc().toISOString()
        if ($("#audit_to").val()) values.to = moment($("#audit_to").val()).utc().toISOString()
        return values
    }
    function queryString(values) { return $.param(values) }
    function loadAudit() {
        $.getJSON("/api/audit/?" + queryString(filters())).done(function (response) {
            total = response.total
            var body = $("#auditTable tbody").empty()
            response.events.forEach(function (event) {
                $("<tr>").append(
                    $("<td>").text(moment.utc(event.timestamp).local().format("YYYY-MM-DD HH:mm:ss")),
                    $("<td>").text(event.actor + (event.actor_id ? " (#" + event.actor_id + ")" : "")),
                    $("<td>").text(event.action),
                    $("<td>").text(event.target_type + ":" + event.target_id),
                    $("<td>").text(event.result),
                    $("<td>").text(event.request_id),
                    $("<td>").text(event.source_ip || "")
                ).appendTo(body)
            })
            $("#audit_page").text("Page " + page + " of " + Math.max(1, Math.ceil(total / perPage)))
            $("#audit_prev").prop("disabled", page <= 1)
            $("#audit_next").prop("disabled", page * perPage >= total)
            var exportFilters = filters(); delete exportFilters.page; delete exportFilters.per_page
            $("#audit_csv").attr("href", "/api/audit/export?format=csv&" + queryString(exportFilters))
            $("#audit_json").attr("href", "/api/audit/export?format=json&" + queryString(exportFilters))
        }).fail(function () { errorFlash("Unable to load audit events") })
    }
    $("#auditFilters").submit(function () { page = 1; loadAudit(); return false })
    $("#audit_prev").click(function () { if (page > 1) page--; loadAudit() })
    $("#audit_next").click(function () { if (page * perPage < total) page++; loadAudit() })
    loadAudit()
})

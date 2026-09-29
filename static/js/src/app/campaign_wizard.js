(function () {
    "use strict";

    var step = 1;
    var maxStep = 4;
    var originalLaunch = window.launch;

    function flashes() {
        return $("#modal\\.flashes");
    }

    function showError(message) {
        flashes().empty().append(
            $("<div>").addClass("alert alert-danger text-center")
                .append($("<i>").addClass("fa fa-exclamation-circle"))
                .append(document.createTextNode(" " + message))
        );
    }

    function clearError() {
        flashes().empty();
    }

    function select2Data(selector) {
        var element = $(selector);
        if (!element.length || !element.hasClass("select2-hidden-accessible")) return [];
        return element.select2("data") || [];
    }

    function selectText(selector) {
        var data = select2Data(selector);
        return data.length ? data[0].text : "";
    }

    function groupNames() {
        return select2Data("#users").map(function (group) {
            return group.text;
        });
    }

    function controlledElements() {
        return $(
            "label[for='name'],#name," +
            "label[for='template'],#template,#template + .select2-container," +
            "label[for='page'],#page,#page + .select2-container," +
            "label[for='url'],#url," +
            "label[for='credential_mode'],#credential_mode,#credential_mode + .help-block," +
            "label[for='credential_disallowed_patterns'],#credential_disallowed_patterns," +
            "label[for='profile'],#profile,#profile + .select2-container," +
            "label[for='users'],#users,#users + .select2-container"
        ).add($("#credential_retention_hours").closest(".row"))
         .add($("#credential_uppercase").closest(".row"))
         .add($("#launch_date").closest(".row"))
         .add($("#profile").closest(".input-group"));
    }

    function elementsForStep(current) {
        if (current === 1) {
            return $(
                "label[for='name'],#name," +
                "label[for='template'],#template,#template + .select2-container," +
                "label[for='page'],#page,#page + .select2-container," +
                "label[for='url'],#url"
            );
        }
        if (current === 2) {
            return $("label[for='credential_mode'],#credential_mode,#credential_mode + .help-block," +
                "label[for='credential_disallowed_patterns'],#credential_disallowed_patterns")
                .add($("#credential_retention_hours").closest(".row"))
                .add($("#credential_uppercase").closest(".row"));
        }
        if (current === 3) {
            return $("label[for='profile'],#profile,#profile + .select2-container," +
                "label[for='users'],#users,#users + .select2-container")
                .add($("#launch_date").closest(".row"))
                .add($("#profile").closest(".input-group"));
        }
        return $();
    }

    function renderStep() {
        controlledElements().hide();
        elementsForStep(step).show();
        $("#campaignReview").prop("hidden", step !== 4);

        $("#campaignWizardSteps li").each(function () {
            var itemStep = parseInt($(this).attr("data-step"), 10);
            $(this).toggleClass("active", itemStep === step);
        });

        // Bootstrap's .btn display rule overrides the browser's [hidden] rule.
        $("#campaignWizardBack").prop("hidden", step === 1).toggle(step !== 1);
        $("#campaignWizardNext").prop("hidden", step === maxStep).toggle(step !== maxStep);
        $("#launchButton").prop("hidden", step !== maxStep).toggle(step === maxStep);

        if (step === maxStep) populateReview();
    }

    function validateBasics() {
        if (!$.trim($("#name").val())) {
            showError("Enter a campaign name before continuing.");
            return false;
        }
        if (!selectText("#template")) {
            showError("Select an email template before continuing.");
            return false;
        }
        if (!selectText("#page")) {
            showError("Select a landing page before continuing.");
            return false;
        }
        if (!$.trim($("#url").val())) {
            showError("Enter the campaign listener URL before continuing.");
            return false;
        }
        return true;
    }

    function validateCredentialPolicy() {
        var minLength = parseInt($("#credential_min_length").val(), 10);
        var maxLength = parseInt($("#credential_max_length").val(), 10);
        var retention = parseInt($("#credential_retention_hours").val(), 10);
        if (!Number.isInteger(minLength) || !Number.isInteger(maxLength) || minLength < 1 || maxLength < minLength || maxLength > 4096) {
            showError("Credential length policy is invalid. Maximum length must be at least the minimum length.");
            return false;
        }
        if (!Number.isInteger(retention) || retention < 0 || retention > 720) {
            showError("Encrypted credential retention must be between 0 and 720 hours.");
            return false;
        }
        return true;
    }

    function validateDelivery() {
        if (!$.trim($("#launch_date").val())) {
            showError("Choose a launch date before continuing.");
            return false;
        }
        if (!selectText("#profile")) {
            showError("Select a sending profile before continuing.");
            return false;
        }
        if (!groupNames().length) {
            showError("Select at least one target group before continuing.");
            return false;
        }
        return true;
    }

    function validateStep(current) {
        clearError();
        if (current === 1) return validateBasics();
        if (current === 2) return validateCredentialPolicy();
        if (current === 3) return validateDelivery();
        return validateBasics() && validateCredentialPolicy() && validateDelivery();
    }

    function populateReview() {
        $("#reviewName").text($("#name").val() || "—");
        $("#reviewTemplate").text(selectText("#template") || "—");
        $("#reviewPage").text(selectText("#page") || "—");
        $("#reviewUrl").text($("#url").val() || "—");
        $("#reviewCredentialMode").text($("#credential_mode option:selected").text() || "—");
        $("#reviewCredentialRetention").text($("#credential_retention_hours").val() + " hours");
        $("#reviewCredentialLength").text($("#credential_min_length").val() + "–" + $("#credential_max_length").val() + " characters");
        $("#reviewCredentialCharacters").text(
            "upper " + $("#credential_uppercase").val() +
            ", lower " + $("#credential_lowercase").val() +
            ", digits " + $("#credential_digit").val() +
            ", symbols " + $("#credential_symbol").val()
        );
        $("#reviewCredentialPatterns").text($.trim($("#credential_disallowed_patterns").val()) || "None");
        $("#reviewProfile").text(selectText("#profile") || "—");
        $("#reviewGroups").text(groupNames().join(", ") || "—");
        $("#reviewLaunchDate").text($("#launch_date").val() || "—");
        $("#reviewSendByDate").text($("#send_by_date").val() || "Not set");
    }

    function resetWizard() {
        step = 1;
        clearError();
        renderStep();
    }

    $("#campaignWizardNext").on("click", function () {
        if (!validateStep(step)) return;
        if (step < maxStep) step += 1;
        renderStep();
    });

    $("#campaignWizardBack").on("click", function () {
        clearError();
        if (step > 1) step -= 1;
        renderStep();
    });

    $("#modal").on("shown.bs.modal", function () {
        resetWizard();
    });

    $("#modal").on("hidden.bs.modal", function () {
        step = 1;
    });

    $(document).ajaxComplete(function () {
        if ($("#modal").hasClass("in")) renderStep();
    });

    window.launch = function () {
        if (step !== maxStep) {
            showError("Complete the campaign review before launching.");
            return;
        }
        if (!validateStep(maxStep)) {
            step = 1;
            renderStep();
            return;
        }
        return originalLaunch.apply(this, arguments);
    };

    $(document).ready(function () {
        renderStep();
    });
})();

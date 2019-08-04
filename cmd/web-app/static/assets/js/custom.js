
$(document).ready(function() {
    hideDuplicateValidationFieldErrors();
});

// Prevent duplicate validation messages. When the validation error is displayed inline
// when the form value, don't display the form error message at the top of the page.
function hideDuplicateValidationFieldErrors() {
    var fieldErrors = 0;

    $(document).find('#page-content form').find('input, select, textarea').each(function(index){
        var fname = $(this).attr('name');
        if (fname === undefined) {
            return;
        }

        var vnode = $(this).parent().find('div.invalid-feedback');
        if (vnode.length == 0) {
            vnode = $(this).parent().parent().find('div.invalid-feedback');
        }

        var formField = $(vnode).attr('data-field');
        var foundMatch = false;
        $(document).find('div.validation-error').find('li').each(function(){
            if ($(this).attr('data-form-field') == formField) {
                foundMatch = true ;

                if ($(vnode).is(":visible") || $(vnode).css('display') === 'none') {
                    $(this).hide();

                    fieldErrors++;
                } else {
                    console.log('form validation feedback for '+fname+' is not visable, display main.');
                }
            }
        });

        // If there was no matching inline validation message, then still need to display the error.
        if (!foundMatch) {
            fieldErrors++;
        }
    });

    if (fieldErrors == 0) {
        $(document).find('div.validation-error').find('ul').hide();
    }
}
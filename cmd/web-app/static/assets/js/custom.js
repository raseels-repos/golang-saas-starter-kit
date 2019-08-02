
$(document).ready(function() {

    // Prevent duplicate validation messages. When the validation error is displayed inline
    // when the form value, don't display the form error message at the top of the page.
    $(this).find('#page-content form').find('input, select, textarea').each(function(index){
        var fname = $(this).attr('name');
        if (fname === undefined) {
            return;
        }

        var vnode = $(this).parent().find('div.invalid-feedback');
        var formField = $(vnode).attr('data-field');
        $(document).find('div.validation-error').find('li').each(function(){
            if ($(this).attr('data-form-field') == formField) {
                if ($(vnode).is(":visible")) {
                    $(this).hide();
                } else {
                    console.log('form validation feedback for '+fname+' is not visable, display main.');
                }
            }
        });
    });

});

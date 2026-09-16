'use strict';
document.querySelectorAll('.dp-limit').forEach((field) => {
 const checkbox = field.querySelector('input[type="checkbox"]');
 const number = field.querySelector('input[type="number"]');
 if (!checkbox || !number) return;
 const update = () => { number.disabled = checkbox.checked; };
 checkbox.addEventListener('change', update);
 update();
});

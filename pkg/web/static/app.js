(function () {
  var tt = document.getElementById('theme-toggle');
  if (tt) {
    tt.addEventListener('click', function () {
      var cur = document.documentElement.dataset.theme;
      var matchDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
      var next;
      if (cur === 'dark') next = 'light';
      else if (cur === 'light') next = 'dark';
      else next = matchDark ? 'light' : 'dark';
      document.documentElement.dataset.theme = next;
      localStorage.setItem('theme', next);
    });
  }

  var dz = document.querySelector('.drop-zone');
  var fi = document.getElementById('upload-files');
  var summary = document.getElementById('file-summary');
  if (dz && fi) {
    function updateSummary() {
      var count = fi.files ? fi.files.length : 0;
      if (!summary) return;
      if (!count) summary.textContent = 'Multiple files supported · 5 GB max';
      else if (count === 1) summary.textContent = fi.files[0].name;
      else summary.textContent = count + ' files selected';
    }
    ['dragenter', 'dragover'].forEach(function (ev) {
      dz.addEventListener(ev, function (e) { e.preventDefault(); dz.classList.add('is-dragging'); });
    });
    ['dragleave', 'drop'].forEach(function (ev) {
      dz.addEventListener(ev, function (e) { e.preventDefault(); dz.classList.remove('is-dragging'); });
    });
    dz.addEventListener('drop', function (e) {
      if (e.dataTransfer && e.dataTransfer.files) { fi.files = e.dataTransfer.files; updateSummary(); }
    });
    fi.addEventListener('change', updateSummary);
  }
})();

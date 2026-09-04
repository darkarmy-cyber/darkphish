const path = require('path');

module.exports = {
    context: path.resolve(__dirname, 'static', 'js', 'src', 'app'),
    entry: {
        passwords: './passwords'
    },
    output: {
        path: path.resolve(__dirname, 'static', 'js', 'dist', 'app'),
        filename: '[name].min.js'
    },
    devtool: false
}

$(document).ready(() => {
    const searchBtn = document.getElementById('wikibook-search-btn')
    const searchText = document.getElementById('wikibook-search-text')
    searchBtn.addEventListener('click', function(e) {
        const inp = searchText.value
        document.getElementById('search-results-title').innerText = `Search results for '${inp}'`
        const searchDiv = document.getElementById('searchDisablable')
        disableElement(searchDiv)
        fetchWrapper(`${addrRoot}/capstone/findtoken?search=${inp}`, 'GET', {}).then((resp) => {
            if (resp.status === 200) {
                document.getElementById('search-results-div').classList.remove('hidden')
                createResultsList(document.getElementById('result-span'), resp.body, 50)
            } else {
                alert(resp.body)
            }
            enableElement(searchDiv)
        })
    })

    Promise.all([
        fetchWrapper(`${addrRoot}/capstone/charts/uniquewords`, 'GET', {}).then((resp) => {
            setInnerHTML(document.getElementById('word-count-chart'), resp.body)
        }),
        fetchWrapper(`${addrRoot}/capstone/charts/pagecounts`, 'GET', {}).then((resp) => {
            setInnerHTML(document.getElementById('chapter-count-chart'), resp.body)
        }),
        fetchWrapper(`${addrRoot}/capstone/charts/externallinks`, 'GET', {}).then((resp) => {
            setInnerHTML(document.getElementById('link-count-chart'), resp.body)
        })
    ]).then()

    const loadPrescriptiveBtn = document.createElement('button')
    loadPrescriptiveBtn.innerText = 'Load Prescriptive Chart'
    loadPrescriptiveBtn.classList.add('button', 'centered')
    loadPrescriptiveBtn.addEventListener('click', () => {
        fetchWrapper(`${addrRoot}/capstone/charts/prescriptive`, 'GET', {}).then((resp) => {
            renderPrescriptive(JSON.parse(resp.body))
        })
    })
    document.getElementById('prescriptive-chart').appendChild(loadPrescriptiveBtn)
})

const setInnerHTML = function(elem, html) {
    elem.innerHTML = html;
    Array.from(elem.querySelectorAll("script")).forEach( oldScript => {
        const newScript = document.createElement("script");
        Array.from(oldScript.attributes)
            .forEach( attr => newScript.setAttribute(attr.name, attr.value) );
        newScript.appendChild(document.createTextNode(oldScript.innerHTML));
        oldScript.parentNode.replaceChild(newScript, oldScript);
    });
}

function createResultsList(elem, arr, maxNumEl) {
    elem.innerHTML = ''
    const resultArr = JSON.parse(arr)
    resultArr.forEach(function(v, i, a) {
        if(i <= maxNumEl) {
            v.book = v.book[0]

            const resSpan = document.createElement('span')
            const ln = document.createElement('a')
            const infoSpan = document.createElement('span')
            const similarBtn = document.createElement('button')
            ln.href = v.book.url
            ln.title = v.book.url
            ln.innerText = `${v.book.title.replace('Wikibooks: ', '')}`

            infoSpan.innerText = `Token: ${v.token}, Qty Occurrences: ${v.qty}`

            similarBtn.classList.add('button')
            similarBtn.innerText = 'Find Similar'
            similarBtn.id = v.docId
            similarBtn.setAttribute('book-title', v.book.title.replace('Wikibooks: ', ''))

            similarBtn.addEventListener('click', function() {
                const lookupId = this.id
                const bookTitle = this.getAttribute('book-title')
                document.getElementById('similarity-results-title').innerText = `Similarity results for '${bookTitle}'`
                const similarityDiv = document.getElementById('similarityDisablable')
                disableElement(similarityDiv)
                fetchWrapper(`${addrRoot}/capstone/findsimilar?id=${lookupId}`, 'GET', {}).then((resp) => {
                    similarityResults(JSON.parse(resp.body))
                    enableElement(similarityDiv)
                })
            })

            resSpan.append(ln, infoSpan, similarBtn)
            elem.appendChild(resSpan)
        }
    })
}

function similarityResults(results) {
    const simSpan = document.getElementById('similar-docs-span')
    document.getElementById('similarityDiv').classList.remove('hidden')
    simSpan.innerHTML = ''
    for (const i in results) {
        v = results[i]
        const a = document.createElement('a')
        a.href = v.url
        a.innerText = v.title
        a.title = `Similarity: ${(v.similarity*100).toFixed(2)}`
        simSpan.appendChild(a)
    }
}

function fetchWrapper(url, method, body) {
    return new Promise((resolve, reject) => {
            if (method === 'GET' || method === 'HEAD') {
                fetch(url).then((response) => returnTheValue(response, resolve))
            } else {
                fetch(url, {
                    method: method,
                    body: JSON.stringify(body)
                }).then((response) => returnTheValue(response, resolve))
            }
        }
    )
}

function returnTheValue(response, resolve) {
    response.text().then((responseBody) => resolve({ status: response.status, body: responseBody }))
}

function disableElement(el) {
    const overlayDiv = document.createElement('div')
    overlayDiv.classList.add('blockingOverlay')
    overlayDiv.id = 'blockingOverlay'
    const overlayP = document.createElement('p')
    overlayP.innerText = 'Working...'
    overlayP.id = 'blockingOverlayP'
    overlayP.classList.add('blockingOverlayP')
    overlayP.classList.add('centered')

    el.prepend(overlayP, overlayDiv)
}

function enableElement(el) {
    $(el).children('.blockingOverlay').remove()
    $(el).children('.blockingOverlayP').remove()
}
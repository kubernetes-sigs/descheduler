#!/bin/bash

# download the code and all its dependencies 
_go_get() {
    go get -u \
       "github.com/ugorji/go/codec" "github.com/ugorji/go/codec"/codecgen \
       github.com/tinylib/msgp/msgp github.com/tinylib/msgp \
       github.com/pquerna/ffjson/ffjson github.com/pquerna/ffjson \
       github.com/Sereal/Sereal/Go/sereal \
       bitbucket.org/bodhisnarkva/cbor/go \
       github.com/davecgh/go-xdr/xdr2 \
       gopkg.in/mgo.v2/bson \
       gopkg.in/vmihailenco/msgpack.v2 \
       github.com/json-iterator/go \
       go.mongodb.org/mongo-driver/bson \
       github.com/mailru/easyjson/...
}

# add generated tag to the top of each file
_prependbt() {
    cat > ${2} <<EOF
// +build generated

EOF
    cat ${1} >> ${2}
    rm -f ${1}
}

# To run the full suite of benchmarks, including executing against the external frameworks
# listed above, you MUST first run code generation for the frameworks that support it.
#
# If you want to run the benchmarks against code generated values.
# Then first generate the code generated values from values_test.go named typed.
# we cannot normally read a _test.go file, so temporarily copy it into a readable file.
_gen() {
    local zsfx="_generated_test.go"
    # local z=`pwd`
    # z=${z%%/src/*}
    # Note: ensure you run the codecgen for this codebase
    cp values_test.go v.go &&
        echo "codecgen ..." &&
        codecgen -nx -rt codecgen -t 'codecgen generated' -o values_codecgen${zsfx} -d 19780 v.go &&
        echo "msgp ... " &&
        msgp -unexported -tests=false -o=m9.go -file=v.go &&
        _prependbt m9.go values_msgp${zsfx} &&
        echo "easyjson ... " &&
        easyjson -all -no_std_marshalers -omit_empty -output_filename e9.go v.go &&
        _prependbt e9.go values_easyjson${zsfx} &&
        echo "ffjson ... " && 
        ffjson -force-regenerate -reset-fields -w f9.go v.go &&
        _prependbt f9.go values_ffjson${zsfx} &&
        sed -i '' -e 's+ MarshalJSON(+ _MarshalJSON(+g' values_ffjson${zsfx} &&
        sed -i '' -e 's+ UnmarshalJSON(+ _UnmarshalJSON(+g' values_ffjson${zsfx} &&
        rm -f easyjson-bootstrap*.go ffjson-inception* &&
        rm -f v.go &&
        echo "... DONE"
}

# run the full suite of tests
#
# Basically, its a sequence of
# go test -tags "alltests x safe codecgen generated" -bench "CodecSuite or AllSuite or XSuite" -benchmem
#

_suite_tests() {
    if [[ "${do_x}" = "1" ]]; then
        printf "\n==== X Baseline ====\n"
        go test "${zargs[@]}" -tags x -v
    else
        printf "\n==== Baseline ====\n"
        go test "${zargs[@]}" -v
    fi
    if [[ "${do_x}" = "1" ]]; then
        printf "\n==== X Generated ====\n"
        go test "${zargs[@]}" -tags "x generated" -v
    else
        printf "\n==== Generated ====\n"
        go test "${zargs[@]}" -tags "generated" -v
    fi
}

_suite_tests_strip_file_line() {
    # sed -e 's/^\([^a-zA-Z0-9]\+\)[a-zA-Z0-9_]\+\.go:[0-9]\+:/\1/'
    sed -e 's/[a-zA-Z0-9_]*.go:[0-9]*://g'
}

_suite_any() {
    local x="$1"
    local g="$2"
    local b="$3"
    shift; shift; shift
    local a=( "" "safe"  "notfastpath" "notfastpath safe" "codecgen" "codecgen safe")
    if [[ "$g" = "g" ]]; then a=( "generated" "generated safe"); fi
    for i in "${a[@]}"; do
        echo ">>>> bench TAGS: 'alltests $x $i' SUITE: $b"
        go test "${zargs[@]}" -tags "alltests $x $i" -bench "$b" -benchmem "$@"
    done 
}

# _suite() {
#     local t="alltests x"
#     local a=( "" "safe"  "notfastpath" "notfastpath safe" "codecgen" "codecgen safe")
#     for i in "${a[@]}"
#     do
#         echo ">>>> bench TAGS: '$t $i' SUITE: BenchmarkCodecXSuite"
#         go test "${zargs[@]}" -tags "$t $i" -bench BenchmarkCodecXSuite -benchmem "$@"
#     done
# }

# _suite_gen() {
#     local t="alltests x"
#     local b=( "generated" "generated safe")
#     for i in "${b[@]}"
#     do
#         echo ">>>> bench TAGS: '$t $i' SUITE: BenchmarkCodecXGenSuite"
#         go test "${zargs[@]}" -tags "$t $i" -bench BenchmarkCodecXGenSuite -benchmem "$@"
#     done
# }

# _suite_json() {
#     local t="alltests x"
#     local a=( "" "safe"  "notfastpath" "notfastpath safe" "codecgen" "codecgen safe")
#     for i in "${a[@]}"
#     do
#         echo ">>>> bench TAGS: '$t $i' SUITE: BenchmarkCodecQuickAllJsonSuite"
#         go test "${zargs[@]}" -tags "$t $i" -bench BenchmarkCodecQuickAllJsonSuite -benchmem "$@"
#     done
# }

# _suite_very_quick_json() {
#     # Quickly get numbers for json, stdjson, jsoniter and json (codecgen)"
#     echo ">>>> very quick json bench"
#     go test "${zargs[@]}" -tags "alltests x" -bench "__(Json|Std_Json|JsonIter)__" -benchmem "$@"
#     echo
#     go test "${zargs[@]}" -tags "alltests codecgen" -bench "__Json____" -benchmem "$@"
# }

_suite_very_quick_json_via_suite() {
    # Quickly get numbers for json, stdjson, jsoniter and json (codecgen)"
    echo ">>>> very quick json bench"
    local prefix="BenchmarkCodecVeryQuickAllJsonSuite/json-all-bd1......../"
    go test "${zargs[@]}" -tags "alltests x" -bench BenchmarkCodecVeryQuickAllJsonSuite -benchmem "$@" |
        sed -e "s+^$prefix++"
    echo "---- CODECGEN RESULTS ----"
    go test "${zargs[@]}" -tags "x generated" -bench "__(Json|Easyjson)__" -benchmem "$@"
}

_suite_very_quick_json_non_suite() {
    # Quickly get numbers for json, stdjson, jsoniter and json (codecgen)"
    echo ">>>> very quick json bench"
    for j in "En" "De"; do
        echo "---- codecgen ----"
        # go test "${zargs[@]}" -tags "generated" -bench "__(Json|Easyjson)__.*${j}" -benchmem "$@"
        go test "${zargs[@]}" -tags "x generated" -bench "__(Json|Easyjson)__.*${j}" -benchmem "$@"
        echo "---- no codecgen ----"
        # go test "${zargs[@]}" -tags "" -bench "__(Json|Std_Json|JsonIter)__.*${j}" -benchmem "$@"
        go test "${zargs[@]}" -tags "x" -bench "__(Json|Std_Json|JsonIter)__.*${j}" -benchmem "$@"
        echo
    done
}

_suite_very_quick_json_only_profile() {
    local a="Json"
    case "$1" in
        Json|Cbor|Msgpack|Simple|Binc) a="${1}"; shift ;;
    esac
    local b="${1}"
    go test "${zargs[@]}" -tags "alltests" -bench "__${a}__.*${b}" \
       -benchmem -benchtime 4s \
       -cpuprofile cpu.out -memprofile mem.out -memprofilerate 1
}

_suite_trim_output() {
    grep -v -E "^(goos:|goarch:|pkg:|PASS|ok|=== RUN|--- PASS)"
}

_usage() {
    printf "usage: bench.sh -[dcbsgjqp] for \n"
    printf "\t-d download\n"
    printf "\t-c code-generate\n"
    printf "\t-tx tests (show stats for each format and whether encoded == decoded); if x, do external also\n"
    printf "\t-sgx run test suite for codec; if g, use generated files; if x, do external also\n"
    printf "\t-jqp run test suite for [json, json-quick, json-profile]\n"
}

_main() {
    if [[ "$1" == "" || "$1" == "-h" || "$1" == "-?" ]]
    then
        _usage
        return 1
    fi
    local zargs=("-count" "1")
    local args=()
    local do_x="0"
    local do_g="0"
    while getopts "dcbsjqptxklg" flag
    do
        case "$flag" in
            d|c|b|s|j|q|p|t|x|k|l|g) args+=( "$flag" ) ;;
            *) _usage; return 1 ;;
        esac
    done
    shift "$((OPTIND-1))"
    
    [[ " ${args[*]} " == *"x"* ]] && do_x="1"
    [[ " ${args[*]} " == *"g"* ]] && do_g="1"
    [[ " ${args[*]} " == *"k"* ]] && zargs+=("-gcflags" "all=-B")
    [[ " ${args[*]} " == *"l"* ]] && zargs+=("-gcflags" "all=-l=4")
    [[ " ${args[*]} " == *"d"* ]] && _go_get "$@"
    [[ " ${args[*]} " == *"c"* ]] && _gen "$@"
    
    [[ " ${args[*]} " == *"s"* && "${do_x}" == 0 && "${do_g}" == 0 ]] && _suite_any - - BenchmarkCodecSuite "$@" | _suite_trim_output
    [[ " ${args[*]} " == *"s"* && "${do_x}" == 0 && "${do_g}" == 1 ]] && _suite_any - g BenchmarkCodecSuite "$@" | _suite_trim_output
    [[ " ${args[*]} " == *"s"* && "${do_x}" == 1 && "${do_g}" == 0 ]] && _suite_any x - BenchmarkCodecXSuite "$@" | _suite_trim_output
    [[ " ${args[*]} " == *"s"* && "${do_x}" == 1 && "${do_g}" == 1 ]] && _suite_any x g BenchmarkCodecXGenSuite "$@" | _suite_trim_output
    
    [[ " ${args[*]} " == *"j"* ]] && _suite_any x - BenchmarkCodecQuickAllJsonSuite "$@" | _suite_trim_output
    [[ " ${args[*]} " == *"q"* ]] && _suite_very_quick_json_non_suite "$@" | _suite_trim_output
    [[ " ${args[*]} " == *"p"* ]] && _suite_very_quick_json_only_profile "$@" | _suite_trim_output
    [[ " ${args[*]} " == *"t"* ]] && _suite_tests "$@" | _suite_trim_output | _suite_tests_strip_file_line
    
    true
    # shift $((OPTIND-1))
}

if [ "." = `dirname $0` ]
then
    _main "$@"
else
    echo "bench.sh must be run from the directory it resides in"
    _usage
fi 

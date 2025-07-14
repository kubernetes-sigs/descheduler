#!/bin/bash

# This script generates release zips and RPMs into _output/releases.
# tito and other build dependencies are required on the host. We will
# be running `hack/build-cross.sh` under the covers, so we transitively
# consume all of the relevant envars.
source "$(dirname "${BASH_SOURCE}")/lib/init.sh"

function cleanup() {
	return_code=$?
	os::util::describe_return_code "${return_code}"
	exit "${return_code}"
}
trap "cleanup" EXIT

# check whether we are in a clean output state
dirty="$( if [[ -d "${OS_OUTPUT}" ]]; then echo '1'; fi )"

os::util::ensure::system_binary_exists rpmbuild
os::util::ensure::system_binary_exists createrepo
os::build::setup_env

if [[ "${OS_ONLY_BUILD_PLATFORMS:-}" == 'linux/amd64' ]]; then
	# when the user is asking for only Linux binaries, we will
	# furthermore not build cross-platform clients in tito
	make_redistributable=0
else
	make_redistributable=1
fi
if [[ -n "${OS_BUILD_SRPM-}" ]]; then
	srpm="a"
else
	srpm="b"
fi


os::build::rpm::get_nvra_vars

OS_RPM_SPECFILE="$( find "${OS_ROOT}" -name *.spec )"
OS_RPM_NAME="$( rpmspec -q --qf '%{name}\n' "${OS_RPM_SPECFILE}" | head -1 )"

os::log::info "Building release RPMs for ${OS_RPM_SPECFILE} ..."

rpm_tmp_dir="${BASETMPDIR}/rpm"

# RPM requires the spec file be owned by the invoking user
chown "$(id -u):$(id -g)" "${OS_RPM_SPECFILE}" || true

if [[ -n "${dirty}" && "${OS_GIT_TREE_STATE}" == "dirty" ]]; then
	os::log::warning "Repository is not clean, performing fast build and reusing _output"

	# build and output from source to destination
	mkdir -p "${rpm_tmp_dir}"
	ln -fs "${OS_ROOT}" "${rpm_tmp_dir}/SOURCES"
	ln -fs "${OS_ROOT}" "${rpm_tmp_dir}/BUILD"

	rpmbuild -bb "${OS_RPM_SPECFILE}" \
		--define "_sourcedir ${rpm_tmp_dir}/SOURCES" \
		--define "_builddir ${rpm_tmp_dir}/BUILD" \
		--define "skip_prep 1" \
		--define "skip_dist 1" \
		--define "make_redistributable ${make_redistributable}" \
		--define "version ${OS_RPM_VERSION}" --define "release ${OS_RPM_RELEASE}" \
		--define "commit ${OS_GIT_COMMIT}" \
		--define "os_git_vars ${OS_RPM_GIT_VARS}" \
		--define 'dist .el7' --define "_topdir ${rpm_tmp_dir}"
	
	mkdir -p "${OS_OUTPUT_RPMPATH}"
	mv -f "${rpm_tmp_dir}"/RPMS/*/*.rpm "${OS_OUTPUT_RPMPATH}"

else
	mkdir -p "${rpm_tmp_dir}/SOURCES"
	tar czf "${rpm_tmp_dir}/SOURCES/${OS_RPM_NAME}-${OS_RPM_VERSION}.tar.gz" \
		--owner=0 --group=0 \
		--exclude=_output --exclude=.git --transform "s|^|${OS_RPM_NAME}-${OS_RPM_VERSION}/|rSH" \
		.

	rpmbuild -b${srpm} "${OS_RPM_SPECFILE}" \
		--define "skip_dist 1" \
		--define "make_redistributable ${make_redistributable}" \
		--define "version ${OS_RPM_VERSION}" --define "release ${OS_RPM_RELEASE}" \
		--define "commit ${OS_GIT_COMMIT}" \
		--define "os_git_vars ${OS_RPM_GIT_VARS}" \
		--define 'dist .el7' --define "_topdir ${rpm_tmp_dir}"

	output_directory="$( find "${rpm_tmp_dir}" -type d -path "*/BUILD/${OS_RPM_NAME}-${OS_RPM_VERSION}/_output/local" )"
	if [[ -z "${output_directory}" ]]; then
		os::log::fatal 'No _output artifact directory found in rpmbuild artifacts!'
	fi

	# migrate the rpm artifacts to the output directory, must be clean or move will fail
	make clean
	mkdir -p "${OS_OUTPUT}"

	# mv exits prematurely with status 1 in the following scenario: running as root,
	# attempting to move a [directory tree containing a] symlink to a destination on
	# an NFS volume exported with root_squash set.  This can occur when running this
	# script on a Vagrant box.  The error shown is "mv: failed to preserve ownership
	# for $FILE: Operation not permitted".  As a workaround, if
	# ${tito_output_directory} and ${OS_OUTPUT} are on different devices, use cp and
	# rm instead.
	if [[ $(stat -c %d "${output_directory}") == $(stat -c %d "${OS_OUTPUT}") ]]; then
		mv "${output_directory}"/* "${OS_OUTPUT}"
	else
		cp -R "${output_directory}"/* "${OS_OUTPUT}"
		rm -rf "${output_directory}"/*
	fi

	mkdir -p "${OS_OUTPUT_RPMPATH}"
	if [[ -n "${OS_BUILD_SRPM-}" ]]; then
		mv -f "${rpm_tmp_dir}"/SRPMS/*src.rpm "${OS_OUTPUT_RPMPATH}"
	fi
	mv -f "${rpm_tmp_dir}"/RPMS/*/*.rpm "${OS_OUTPUT_RPMPATH}"
fi

mkdir -p "${OS_OUTPUT_RELEASEPATH}"
echo "${OS_GIT_COMMIT}" > "${OS_OUTPUT_RELEASEPATH}/.commit"

repo_path="$( os::util::absolute_path "${OS_OUTPUT_RPMPATH}" )"
createrepo "${repo_path}"

echo "[${OS_RPM_NAME}-local-release]
baseurl = file://${repo_path}
gpgcheck = 0
name = Release from Local Source for ${OS_RPM_NAME}
enabled = 1
" > "${repo_path}/local-release.repo"

os::log::info "Repository file for \`yum\` or \`dnf\` placed at ${repo_path}/local-release.repo
Install it with:
$ mv '${repo_path}/local-release.repo' '/etc/yum.repos.d"
